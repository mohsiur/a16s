package view

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/aws/aws-sdk-go-v2/aws"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/gdamore/tcell/v2"
	"github.com/mohsiur/a16s/internal/api"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/ui"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

var theme color.Colors
var ErrHandledNavigation = errors.New("navigation already handled")
var globalProfile string
var globalRegion string

// Entity contains ECS resources to show, use uppercase to make items like app.cluster easy to access
type Entity struct {
	cluster           *types.Cluster
	service           *types.Service
	task              *types.Task
	container         *types.Container
	taskDefinition    *types.TaskDefinition
	events            []types.ServiceEvent
	metrics           *api.MetricsData
	autoScaling       *api.AutoScalingData
	instance          *types.ContainerInstance
	serviceDeployment *types.ServiceDeployment
	serviceRevision   *types.ServiceRevision
	profile           string
	region            *api.Region
	entityName        string
	// Flat-kind selection state. Set by changeSelectedValues for the
	// corresponding kind so drill-downs can read it without re-listing.
	lambdaFunction *lambdaTypes.FunctionConfiguration
	sqsQueueName   string
	sqsMessage     *sqsTypes.Message
	ddbTable       *ddbTypes.TableDescription
	ddbIndex       *ddbIndex
}

type Option struct {
	// Read only mode indicator
	ReadOnly bool
	// Reload resources every x second(s), -1 is stop auto refresh
	Refresh int
	// ECS exec shell
	Shell string
	// Here for help view
	Debug bool
	// Here for help view
	JSON bool
	// Here for help view
	LogFile string
	// Here for help view
	ConfigFile string
	// Here for help view
	Theme string
	// Default cluster name
	Cluster string
	// Default service name
	Service string
	// Splash screen on startup (load AWS config and first resource list in background).
	Splash bool
}

// App is the tview application root: it embeds tview.Application and
// tview.Pages, owns the navigation/Notice surface, and holds the active
// Entity selection state used across kind switches.
type App struct {
	// tview Application
	*tview.Application
	// Info + table area pages UI for MainScreen
	*tview.Pages
	// Notice text UI in MainScreen footer
	Notice *ui.Notice
	// mainScreen content UI
	mainScreen *tview.Flex
	// mainScreenFooter is the Notice row at the bottom of mainScreen. Stashed
	// here so showPalette can re-attach it after mounting the `:` input row.
	mainScreenFooter *tview.Flex
	// paletteInput is the active `:` command input, mounted as a 1-row child of
	// mainScreen above Pages. nil when no palette is showing.
	paletteInput *tview.InputField
	// AWS service clients (lazy per-service, lock-protected on switch).
	*api.Clients
	// Option from cli args
	Option
	// Current screen item content, use uppercase to make items like app.cluster easy to access
	Entity
	// Current page primary kind ex: cluster, service
	kind kind
	// Current secondary kind like json, list
	secondaryKind kind
	// Track back kind when necessary
	backKind kind
	// Port forwarding ssm session Id
	sessions []*PortForwardingSession
	// Current primary kind table row index for auto refresh to keep row selected
	rowIndex int
	// Specify in tview app suspend or not
	isSuspended bool
	// Show selected status tasks
	taskStatus types.DesiredStatus
	// Show resources from cluster
	fromCluster bool
	// First paint after splash: avoid a second identical API list call.
	bootstrapClusters []types.Cluster
	bootstrapServices []types.Service
	// Set when splash bootstrap fails before Run() returns; read after Run().
	splashStartupErr error
	// ctx is cancelled in onClose() to stop background goroutines (notably the
	// auto-refresh ticker) when the tview application has stopped. Without
	// this the ticker goroutine would outlive Run() and race with shutdown
	// when calling QueueUpdateDraw.
	ctx    context.Context
	cancel context.CancelFunc
}

func newApp(option Option) (*App, error) {
	globalProfile = os.Getenv("AWS_PROFILE")
	globalRegion = os.Getenv("AWS_REGION")
	var clients *api.Clients
	var err error
	if !option.Splash {
		clients, err = api.NewAWSClients(globalProfile, globalRegion)
		if err != nil {
			return nil, err
		}
	}
	app := tview.NewApplication()
	pages := tview.NewPages()
	footer := tview.NewFlex()

	notice := ui.NewNotice(app, theme)
	footer.AddItem(notice, 0, 1, false)
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pages, 0, 2, true).
		AddItem(footer, 1, 1, false)

	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		Application:      app,
		Pages:            pages,
		Notice:           notice,
		mainScreen:       main,
		mainScreenFooter: footer,
		Clients:          clients,
		Option:        option,
		kind:          ClusterKind,
		secondaryKind: EmptyKind,
		backKind:      EmptyKind,
		taskStatus:    types.DesiredStatusRunning,
		ctx:           ctx,
		cancel:        cancel,
		Entity: Entity{
			cluster: &types.Cluster{
				ClusterName: aws.String("a16s_default_cluster"),
				ClusterArn:  aws.String("a16s_default_cluster_arn"),
			},
			service: &types.Service{
				ServiceName: aws.String("a16s_default_service"),
				ServiceArn:  aws.String("a16s_default_service arn"),
			},
			task:           &types.Task{},
			container:      &types.Container{},
			taskDefinition: &types.TaskDefinition{},
		},
	}, nil
}

// Entry point of the app
func Start(option Option) error {
	file := utils.GetLogger(option.LogFile, option.JSON, option.Debug)
	defer file.Close()
	slog.Debug(`
****************************************************************
**************** Started a16s
****************************************************************`)
	slog.Debug("a16s start", "option", option)
	theme = color.InitStyles(option.Theme)

	app, err := newApp(option)
	if err != nil {
		return err
	}

	api.OnConfigSwitch = kindpkg.ResetAll

	app.SetInputCapture(app.globalInputHandle)

	if option.Splash {
		app.SetRoot(app.buildSplashPage(), true)
		go app.runSplashBootstrap()
		if err := app.Application.Run(); err != nil {
			return err
		}
		if app.splashStartupErr != nil {
			return app.splashStartupErr
		}
	} else {
		// Fire background inventory loads for opt-in flat kinds (Lambda, SQS,
		// DDB). Each Preload runs in its own goroutine.
		kindpkg.PreloadAll(app)
		if err := app.start(); err != nil {
			return err
		}
		if err := app.Application.SetRoot(app.mainScreen, true).Run(); err != nil {
			return err
		}
	}
	app.onClose()
	return nil
}

// Add new page to app.Pages
func (app *App) addAppPage(page *tview.Flex) {
	pageName := app.kind.getAppPageName(app.getPageHandle())

	slog.Debug("app.Pages navigation", "action", "AppPage", "pageName", pageName, "app", app)

	app.Pages.AddPage(pageName, page, true, true)
}

// Switch app.Pages page
func (app *App) switchPage(reload bool) bool {
	pageName := app.kind.getAppPageName(app.getPageHandle())
	if app.Pages.HasPage(pageName) && app.Refresh < 0 && !reload {

		slog.Debug("app.Pages navigation", "action", "SwitchToPage", "pageName", pageName, "app", app)
		app.Pages.SwitchToPage(pageName)
		return true
	}
	return false
}

// Go back page based on current kind
func (app *App) back() {
	slog.Debug("app.Pages back", "kind", app.kind)
	app.taskStatus = types.DesiredStatusRunning

	prevKind := app.kind.prevKind()
	if app.backKind != EmptyKind {
		prevKind = app.backKind
		app.backKind = EmptyKind
	}

	if app.fromCluster && prevKind == ServiceKind {
		app.fromCluster = false
		prevKind = ClusterKind
	}

	app.kind = prevKind
	app.secondaryKind = EmptyKind
	pageName := prevKind.getAppPageName(app.getPageHandle())

	slog.Debug("app.Pages navigation", "action", "back", "pageName", pageName, "app", app)

	if prevKind == ClusterKind && app.Option.Cluster != "" {
		app.Option.Cluster = ""
		err := app.showPrimaryKindPage(ClusterKind, false)
		if err != nil {
			app.Notice.Warn("failed to back to cluster list")
		}
		return
	}

	if prevKind == ServiceKind && app.Option.Service != "" {
		app.Option.Service = ""
		err := app.showPrimaryKindPage(ServiceKind, false)
		if err != nil {
			app.Notice.Warn("failed to back to service list")
		}
		return
	}

	app.Pages.SwitchToPage(pageName)
}

// Get page handler, cluster is empty, other is cluster arn.
//
// Migrated kinds answer through Resource.PageHandle (read from the
// registry's cached parent selection); a non-empty result short-circuits the
// legacy switch. The TaskKind status suffix and fromCluster suffix still
// run unconditionally because they're cross-cutting page-keying concerns
// (the same kind shows under different page names depending on app state),
// not parent-context derivation.
func (app *App) getPageHandle() string {
	name := ""
	if r := resolveResource(app.kind); r != nil {
		name = r.PageHandle()
	}
	if name == "" {
		switch app.kind {
		case ServiceKind:
			name = *app.cluster.ClusterArn
		case TaskKind, TaskDefinitionKind, ServiceDeploymentKind:
			name = *app.service.ServiceArn
		case ContainerKind:
			name = *app.task.TaskArn
		case SQSPeekKind:
			name = app.Entity.sqsQueueName
		case DynamoDBIndexKind:
			if app.Entity.ddbTable != nil {
				name = aws.ToString(app.Entity.ddbTable.TableName)
			}
		case DynamoDBScanKind:
			if app.Entity.ddbTable != nil && app.Entity.ddbIndex != nil {
				name = aws.ToString(app.Entity.ddbTable.TableName) + "." + app.Entity.ddbIndex.name
			}
		}
	}
	// based on different task status different name
	if app.kind == TaskKind {
		name = name + "." + strings.ToLower(string((app.taskStatus)))
	}

	// true when show tasks in cluster
	if app.fromCluster {
		name = name + ".cluster"
	}
	return name
}

func (app *App) start() error {
	var err error
	if app.Option.Cluster == "" {
		err = app.showPrimaryKindPage(ProfileKind, false)
	} else {
		app.cluster.ClusterName = &app.Option.Cluster
		if app.Option.Service == "" {
			err = app.showPrimaryKindPage(ServiceKind, false)
		} else {
			app.service.ServiceName = &app.Option.Service
			err = app.showPrimaryKindPage(TaskKind, false)
		}
	}

	if app.Option.Refresh > 0 {
		slog.Debug("Auto refresh rate", "seconds", app.Option.Refresh)
		ticker := time.NewTicker(time.Duration(app.Option.Refresh) * time.Second)
		ctx := app.ctx

		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				if app.secondaryKind != EmptyKind || app.isSuspended {
					continue
				}
				// Refresh runs in two phases so the AWS round-trip never
				// blocks the tview event loop. Phase 1 (here): refresh the
				// flat-kind cache off the event loop. Phase 2 (below, inside
				// QueueUpdateDraw): rebuild the page from the warm cache.
				// We pass reload=false to showPrimaryKindPage so loadInventory
				// short-circuits on `k.loaded` and we get exactly one network
				// round-trip per tick. ECS show*Pages refetch synchronously,
				// so for those the stutter is unavoidable here without a
				// bigger refactor — accepted because cluster/service/task
				// inventories are small.
				app.preheatKindForRefresh(app.kind)
				app.QueueUpdateDraw(func() {
					// Drop the cached page so switchPage doesn't short-circuit
					// on its existence and skip the rebuild. Without this we
					// would refresh the underlying cache but never see the
					// new rows.
					if pageName := app.kind.getAppPageName(app.getPageHandle()); app.Pages.HasPage(pageName) {
						app.Pages.RemovePage(pageName)
					}
					// showPrimaryKindPage already shows the error in Notice
					_ = app.showPrimaryKindPage(app.kind, false)
					slog.Debug("Auto refresh")
				})
			}
		}()
	}
	return err
}

// preheatKindForRefresh runs the inventory fetch for kind k off the tview
// event loop so the auto-refresh ticker doesn't block scroll input behind a
// multi-second AWS round-trip. Kinds opt in by implementing
// kindpkg.Refresher; ECS kinds fetch inside their show*Page and the
// inventories are small enough that the inline stutter is tolerable, so
// they don't satisfy the interface and this is a no-op for them.
func (app *App) preheatKindForRefresh(k kind) {
	r := resolveResource(k)
	if r == nil {
		return
	}
	if rf, ok := r.(kindpkg.Refresher); ok {
		_ = rf.Refresh(app)
	}
}

// Show Primary kind page
func (app *App) showPrimaryKindPage(k kind, reload bool) error {
	if k == TaskDefinitionKind {
		app.backKind = app.kind
	}
	app.kind = k
	// Resource-aware kinds dispatch through the registry. Legacy kinds fall
	// through to the enum switch when Resource.Show returns
	// ErrShowUnimplemented (the BaseKind default).
	if r := resolveResource(k); r != nil {
		if err := r.Show(app, reload); !errors.Is(err, kindpkg.ErrShowUnimplemented) {
			return app.finishShowPrimaryKindPage(err, reload)
		}
	}
	var err error
	switch k {
	case ProfileKind:
		err = app.showProfilesPage(reload)
	case RegionKind:
		err = app.showRegionsPage(reload)
	case ClusterKind:
		err = app.showClustersPage(reload)
	case InstanceKind:
		err = app.showInstancesPage(reload)
	case ServiceKind:
		err = app.showServicesPage(reload)
	case TaskKind:
		err = app.showTasksPages(reload)
	case ContainerKind:
		err = app.showContainersPage(reload)
	case TaskDefinitionKind:
		err = app.showTaskDefinitionPage(reload)
	case ServiceDeploymentKind:
		err = app.showServiceDeploymentPage(reload)
	default:
		app.kind = ClusterKind
		err = app.showClustersPage(reload)
	}
	return app.finishShowPrimaryKindPage(err, reload)
}

// finishShowPrimaryKindPage applies the post-show side effects that used to
// live at the bottom of showPrimaryKindPage: noisy notices on error, a
// "Viewing X..." breadcrumb on success, debug log on reload. Extracted so the
// Resource dispatch path and the legacy enum switch can share it.
func (app *App) finishShowPrimaryKindPage(err error, reload bool) error {
	if err != nil {
		if errors.Is(err, ErrHandledNavigation) {
			// A valid page has already been shown (for example, fallback from
			// empty clusters to regions), so skip the noisy error notice.
			return nil
		}
		slog.Error("failed to show primary kind page", "error", err)
		app.Notice.Error(err.Error())
		return err
	}
	if !reload {
		if app.taskStatus != types.DesiredStatusStopped {
			app.Notice.Infof("Viewing %s...", app.kind.String())
		}
	} else {
		slog.Debug("Reload in showPrimaryKindPage")
	}
	return nil
}

// app close hook
func (app *App) onClose() {
	// Cancel the app context so background goroutines (notably the
	// auto-refresh ticker) exit before the tview application is fully torn
	// down. Run() has already returned by the time onClose is called, so any
	// further QueueUpdateDraw from a leaked ticker would race with shutdown.
	if app.cancel != nil {
		app.cancel()
	}

	if len(app.sessions) != 0 {
		ids := []*string{}
		for _, s := range app.sessions {
			ids = append(ids, s.sessionId)
		}
		err := app.Clients.TerminateSessions(ids)
		if err != nil {
			slog.Error("Failed to terminated port forwarding sessions", "error", err)
		} else {
			slog.Debug("Terminated port forwarding session terminated")
		}
	}

	slog.Debug(`
**************** Exited a16s ************************************`)
}

func (app *App) globalInputHandle(event *tcell.EventKey) *tcell.EventKey {
	if app.Clients == nil {
		switch event.Key() {
		case tcell.KeyCtrlC:
			return event
		default:
			return nil
		}
	}

	switch event.Rune() {
	case '?':
		app.showHelpPage()
	case ':':
		app.showPalette()
		return nil
	}

	// Handle Ctrl+P for profile switcher
	switch event.Key() {
	case tcell.KeyCtrlP:
		app.kind = ProfileKind
		app.showProfilesPage(false)
	case tcell.KeyCtrlR:
		app.kind = RegionKind
		app.showRegionsPage(false)
	}

	return event
}

func (app *App) LogValue() slog.Value {
	return slog.AnyValue(struct {
		kind          string
		secondaryKind string
		cluster       string
		service       string
	}{
		kind:          app.kind.String(),
		secondaryKind: app.secondaryKind.String(),
		cluster:       *app.cluster.ClusterName,
		service:       *app.service.ServiceName,
	})
}

func (app *App) copyToClipboard(item string, content string) {
	err := clipboard.WriteAll(content)
	if err != nil {
		app.Notice.Error("Failed to copy to clipboard")
	}

	app.Notice.Info(fmt.Sprintf("Copied %s to clipboard", item))
}

// AWSClients returns the embedded api.Clients. Satisfies kind.App. Named
// AWSClients (not Clients) because the *App struct already embeds *api.Clients,
// whose name is promoted as Clients — defining a Clients() method would collide.
func (app *App) AWSClients() *api.Clients { return app.Clients }

// effectiveRegion returns the active AWS region. Prefer the resolved region on
// the loaded SDK config (populated for SSO/shared-config profiles where
// AWS_REGION isn't set), fall back to globalRegion (set from AWS_REGION env or
// the in-app region picker).
func (app *App) effectiveRegion() string {
	if app.Clients != nil {
		if r := app.Clients.Config().Region; r != "" {
			return r
		}
	}
	return globalRegion
}

// FlashError surfaces an error message in the footer notice. Satisfies kind.App.
func (app *App) FlashError(msg string) {
	app.Notice.Warn(msg)
}

