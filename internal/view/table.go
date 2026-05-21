package view

import (
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/gdamore/tcell/v2"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	"github.com/rivo/tview"
)

const (
	L = tview.AlignLeft
	C = tview.AlignCenter
	R = tview.AlignRight
)

// Build common table
func (v *view) buildTable(title string, headers []string, rowsBuilder func() [][]string) {

	v.table.
		SetFixed(5, 5).
		SetSelectable(true, false)

	v.table.
		SetBorder(true).
		SetTitle(title).
		SetBorderPadding(0, 0, 1, 1)

	v.headers = headers
	v.originalRowData = rowsBuilder()

	v.buildTableContent(v.originalRowData, v.originalRowReferences)

	v.handleTableEvents()

	pageName := v.app.kind.getTablePageName(v.app.getPageHandle())
	v.tablePages.AddPage(pageName, v.table, true, true)
}

// Build table content based on headers and sorted row data
func (v *view) buildTableContent(rowData [][]string, references []Entity) {
	// init with first column width
	expansions := []int{2}
	alignment := []int{L}

	for i := 1; i < len(v.headers); i++ {
		expansions = append(expansions, 1)
		alignment = append(alignment, C)
	}

	data := [][]string{v.headers}
	data = append(data, rowData...)

	for y, row := range data {
		for x, text := range row {
			cell := tview.NewTableCell("")
			if y == 0 {
				if x == v.sortColumn {
					if v.sortOrder == "asc" {
						text = text + " ↑"
					} else {
						text = text + " ↓"
					}
				}
				cell.SetTextColor(color.Color(theme.Yellow))
				cell.SetSelectable(false)
			}
			cell.SetText(text).
				SetAlign(alignment[x]).
				SetExpansion(expansions[x]).
				SetMaxWidth(30)
			if y > 0 {
				cell.SetReference(references[y-1])
			}
			v.table.SetCell(y, x, cell)
		}
	}

}

// Handler common table events
func (v *view) handleTableEvents() {
	v.table.SetSelectionChangedFunc(v.handleSelectionChanged)

	v.table.SetSelectedFunc(v.handleSelected)

	v.table.SetInputCapture(v.handleInputCapture)

	v.table.SetDoneFunc(v.handleDone)

	// prevent table row selection out of range
	if v.app.rowIndex >= v.table.GetRowCount() {
		v.app.rowIndex = 1
	}
}

// Handle selected event for table when press up and down
// Detail page will switch
func (v *view) handleSelectionChanged(row, column int) {
	v.changeSelectedValues()
	selected, err := v.getCurrentSelection()
	if err != nil {
		v.app.Notice.Warnf("failed to handleSelectionChanged")
		return
	}
	v.app.rowIndex = row
	v.headerPages.SwitchToPage(selected.entityName)
}

func (v *view) revertProfileOrRegion(to string, prev string) {
	slog.Debug("Reverting profile or region", "to", to, "prev", prev)
	v.app.Pages.SwitchToPage(to)
	if to == "profiles" {
		v.app.kind = ProfileKind
		globalProfile = prev
	} else {
		v.app.kind = RegionKind
		globalRegion = prev
	}
	v.app.Store.SwitchAwsConfig(globalProfile, globalRegion)
}

// Handle selected event for table when press Enter
func (v *view) handleSelected(row, column int) {
	if v.app.kind == ProfileKind {
		cell := v.table.GetCell(row, column)
		cell.GetReference()
		prevProfile := globalProfile
		switch entity := cell.GetReference().(type) {
		case Entity:
			globalProfile = entity.profile
			slog.Info("Handle select", "profile", globalProfile)
			if err := v.app.Store.SwitchAwsConfig(globalProfile, globalRegion); err != nil {
				v.revertProfileOrRegion("profiles", prevProfile)
				return
			}
			err := v.app.showPrimaryKindPage(ClusterKind, false)
			if err != nil {
				v.revertProfileOrRegion("profiles", prevProfile)
				return
			}
			v.app.Notice.Info(fmt.Sprintf("Switched to Profile: %s, Region: %s", globalProfile, globalRegion))
		}
		return
	}
	if v.app.kind == RegionKind {
		cell := v.table.GetCell(row, column)
		cell.GetReference()
		prevRegion := globalRegion
		switch entity := cell.GetReference().(type) {
		case Entity:
			globalRegion = entity.region.Code
			slog.Info("Handle select", "region", globalRegion)
			if err := v.app.Store.SwitchAwsConfig(globalProfile, globalRegion); err != nil {
				v.revertProfileOrRegion("regions", prevRegion)
				return
			}
			err := v.app.showPrimaryKindPage(ClusterKind, false)
			if err != nil {
				v.revertProfileOrRegion("regions", prevRegion)
				return
			}
			v.app.Notice.Info(fmt.Sprintf("Switched to Profile: %s, Region: %s", globalProfile, globalRegion))
		}

		return

	}
	if v.app.kind == TaskDefinitionKind || v.app.kind == InstanceKind {
		return
	}
	if v.app.kind == ContainerKind {
		v.execShell()
	}
	if v.app.kind == LambdaKind {
		// Lambda has no drill-down child; Enter opens the function's log tail.
		v.openLambdaLogs()
		return
	}
	if v.app.kind == SQSPeekKind {
		// SQS messages: Enter opens the message body in a read-only view.
		v.openSQSMessageBody()
		return
	}
	if v.app.kind == DynamoDBScanKind {
		// Scan items are leaf rows; Enter is a no-op until a future drill-in.
		return
	}
	v.app.rowIndex = 0
	v.app.showPrimaryKindPage(v.app.kind.nextKind(), false)
}

// Handle keyboard input
func (v *view) handleInputCapture(event *tcell.EventKey) *tcell.EventKey {
	// If it's single keystroke, event.Rune() is ascii code
	switch event.Rune() {
	case 'a':
		if v.app.kind == ServiceKind {
			v.app.secondaryKind = AutoScalingKind
			v.showSecondaryKindPage(false)
			return event
		}
	case 'b':
		v.openInBrowser()
	case 'c':
		v.app.copyToClipboard("page name", v.app.kind.getTablePageName(v.app.getPageHandle()))
	case 'd':
		v.app.secondaryKind = DescriptionKind
		v.showSecondaryKindPage(false)
	case 'L':
		if v.app.kind == ServiceKind || v.app.kind == TaskKind || v.app.kind == ContainerKind {
			v.app.secondaryKind = LogKind
			v.showSecondaryKindPage(false)
			return event
		}
		if v.app.kind == LambdaKind {
			v.openLambdaLogs()
			return event
		}
	case 'i':
		if v.app.kind == LambdaKind {
			v.invokeLambda()
			return event
		}
	case 'q':
		if v.app.kind == DynamoDBIndexKind {
			v.queryDDBIndex()
			return event
		}
	case 'm':
		if v.app.kind == ServiceKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.serviceMetricsForm, 15)
			return event
		}
	case 't':
		if v.app.kind == ServiceKind || v.app.kind == TaskKind {
			v.showKindPage(TaskDefinitionKind, false)
			return event
		}
	case 'p':
		if v.app.kind == ServiceKind {
			v.showKindPage(ServiceDeploymentKind, false)
			return event
		}
		if v.app.kind == SQSKind {
			v.purgeSelectedQueue()
			return event
		}
	case 'v':
		if v.app.kind == ServiceDeploymentKind {
			v.app.secondaryKind = ServiceRevisionKind
			v.showSecondaryKindPage(false)
			return event
		}
	case 'r':
		v.sortColumn = 0
		v.sortOrder = "desc"
		v.reloadResource(true)
	case 'R':
		if v.app.kind == ServiceDeploymentKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.rollbackServiceDeploymentForm, 6)
			return event
		}
	case 'x':
		if v.app.kind == TaskKind {
			if v.app.taskStatus == types.DesiredStatusRunning {
				v.app.taskStatus = types.DesiredStatusStopped
			} else {
				v.app.taskStatus = types.DesiredStatusRunning
			}
			v.showKindPage(TaskKind, false)
			return event
		}
	case 's':
		if v.app.kind == ContainerKind {
			v.execShell()
		}
		if v.app.kind == InstanceKind || v.app.kind == TaskKind {
			v.instanceStartSession()
		}
		if v.app.kind == SQSKind {
			v.sendTestMessageToQueue()
			return event
		}
		return event
	case 'S':
		if v.app.kind == TaskKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.stopTaskForm, 6)
			return event
		}
	case 'N':
		if v.app.kind == ClusterKind {
			v.app.fromCluster = true
			v.showKindPage(TaskKind, false)
			return event
		}
	case 'n':
		if v.app.kind == ClusterKind {
			v.app.fromCluster = true
			v.showKindPage(InstanceKind, false)
			return event
		}
	case 'w':
		if v.app.kind == ServiceKind {
			v.app.secondaryKind = ServiceEventsKind
			v.showSecondaryKindPage(false)
			return event
		}
	case 'F':
		if v.app.kind == ContainerKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.portForwardingForm, 15)
			return event
		}
	case 'U':
		if v.app.kind == ServiceKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.serviceUpdateForm, 15)
			return event
		}
		if v.app.kind == TaskDefinitionKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.serviceUpdateWithSpecificTaskDefinitionForm, 6)
			return event
		}
	case 'T':
		if v.app.kind == ContainerKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.terminatePortForwardingForm, 6)
			return event
		}
	case 'P':
		if v.app.kind == ContainerKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.cpForm, 15)
			return event
		}
	case 'E':
		if v.app.kind == ContainerKind {
			v.app.secondaryKind = ModalKind
			v.showFormModal(v.execCommandForm, 7)
			return event
		}
	case 'D':
		if v.app.kind == LambdaKind {
			v.openLambdaDLQ()
			return event
		}
		v.app.secondaryKind = ModalKind
		v.showFormModal(v.catFile, 10)
		return event
	case '/':
		v.showFilterInput()
		return event
	case 'h':
		v.handleDone(0)
	case 'l':
		v.handleSelected(0, 0)
	}

	// If it's composite keystroke, event.Key() is ctrl-char ascii code
	switch event.Key() {
	// Handle left arrow key. On leaf flat-kind tables the arrows fall
	// through to tview so the user can scroll horizontally; h/Esc still
	// navigate back from those views.
	case tcell.KeyLeft:
		if !v.app.kind.isFlatLeaf() {
			v.handleDone(0)
		}
	// Handle right arrow key
	case tcell.KeyRight:
		if !v.app.kind.isFlatLeaf() {
			v.handleSelected(0, 0)
		}
	case tcell.KeyCtrlZ:
		v.handleDone(0)
	case tcell.KeyF1:
		v.sortByColumn(0)
	case tcell.KeyF2:
		v.sortByColumn(1)
	case tcell.KeyF3:
		v.sortByColumn(2)
	case tcell.KeyF4:
		v.sortByColumn(3)
	case tcell.KeyF5:
		v.sortByColumn(4)
	case tcell.KeyF6:
		v.sortByColumn(5)
	case tcell.KeyF7:
		v.sortByColumn(6)
	case tcell.KeyF8:
		v.sortByColumn(7)
	case tcell.KeyF9:
		v.sortByColumn(8)
	case tcell.KeyF10:
		v.sortByColumn(9)
	case tcell.KeyF11:
		v.sortByColumn(10)
	case tcell.KeyF12:
		v.sortByColumn(11)
	case tcell.KeyEsc:
		if v.filterInput != nil && v.filterInput.GetText() != "" {
			v.filterInput.SetText("")
			v.applyFilter()
		}
	}

	// slog.Debug("Key stroke", "key", event.Key(), "rune", event.Rune())
	return event
}

// Handle done event for table when press ESC
func (v *view) handleDone(key tcell.Key) {
	if v.app.kind == ProfileKind {
		return
	}
	v.app.back()
}

// Handle common values changing for selected event for table when pressed Enter
func (v *view) changeSelectedValues() {
	selected, err := v.getCurrentSelection()
	if err != nil {
		v.app.Notice.Warnf("failed to changeSelectedValues")
		return
	}
	switch v.app.kind {
	case ProfileKind:
		profile := selected.profile
		if profile != "" {
			v.app.profile = profile
			v.app.entityName = profile
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case RegionKind:
		region := selected.region
		if region != nil {
			v.app.region = region
			v.app.entityName = region.Code
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case ClusterKind:
		cluster := selected.cluster
		if cluster != nil {
			v.app.cluster = cluster
			v.app.entityName = *cluster.ClusterArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case ServiceKind:
		service := selected.service
		if service != nil {
			v.app.service = service
			v.app.entityName = *service.ServiceArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case TaskKind:
		task := selected.task
		if task != nil {

			v.app.task = task
			v.app.entityName = *task.TaskArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case ContainerKind:
		container := selected.container
		if container != nil {
			v.app.container = selected.container
			v.app.entityName = *container.ContainerArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case TaskDefinitionKind:
		taskDefinition := selected.taskDefinition
		if taskDefinition != nil {
			v.app.taskDefinition = selected.taskDefinition
			v.app.entityName = *taskDefinition.TaskDefinitionArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case ServiceDeploymentKind:
		serviceDeployment := selected.serviceDeployment
		if serviceDeployment != nil {
			v.app.serviceDeployment = selected.serviceDeployment
			v.app.entityName = *serviceDeployment.ServiceDeploymentArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case InstanceKind:
		instance := selected.instance
		if instance != nil {
			v.app.instance = selected.instance
			v.app.entityName = *instance.ContainerInstanceArn
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case LambdaKind:
		fn := selected.lambdaFunction
		if fn != nil {
			v.app.lambdaFunction = fn
			v.app.entityName = selected.entityName
			if lk := getLambdaKind(); lk != nil {
				lk.SetSelection(fn)
			}
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case SQSKind:
		if selected.sqsQueueName != "" {
			v.app.sqsQueueName = selected.sqsQueueName
			v.app.entityName = selected.entityName
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case SQSPeekKind:
		if selected.sqsMessage != nil {
			v.app.sqsMessage = selected.sqsMessage
			v.app.entityName = selected.entityName
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case DynamoDBKind:
		if selected.ddbTable != nil {
			v.app.ddbTable = selected.ddbTable
			v.app.entityName = selected.entityName
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case DynamoDBIndexKind:
		if selected.ddbIndex != nil {
			v.app.ddbIndex = selected.ddbIndex
			v.app.entityName = selected.entityName
		} else {
			slog.Warn("unexpected in changeSelectedValues", "kind", v.app.kind)
			return
		}
	case DynamoDBScanKind:
		v.app.entityName = selected.entityName
	default:
		v.app.back()
	}
}

// Open selected resource in browser. ECS kinds are routed through ArnToUrl;
// flat kinds (lambda/sqs/ddb and their drill-downs) build the console URL
// directly from the selected entity + the active region since they aren't
// keyed by ARN.
func (v *view) openInBrowser() {
	selected, err := v.getCurrentSelection()
	if err != nil {
		v.app.Notice.Warnf("failed to openInBrowser")
		return
	}
	arn := ""
	taskService := ""
	url := ""
	region := v.app.effectiveRegion()
	// Migrated kinds answer BrowserURL via kindpkg.Resource. When the migrated
	// path returns a URL we skip the legacy switch entirely; otherwise the
	// switch below is the fallback for kinds still on the enum (Phase 3+).
	if r := resolveResource(v.app.kind); r != nil {
		if u, _ := r.BrowserURL(region); u != "" {
			slog.Info("open", "url", u, "via", "kind.Resource")
			if err := utils.OpenURL(u); err != nil {
				v.app.Notice.Warnf("failed to open url %s\n", u)
			}
			return
		}
	}
	switch v.app.kind {
	case ClusterKind:
		arn = *selected.cluster.ClusterArn
	case ServiceKind:
		arn = *selected.service.ServiceArn
	case TaskKind:
		taskService = *v.app.service.ServiceName
		arn = *selected.task.TaskArn
	case ContainerKind:
		taskService = *v.app.service.ServiceName
		arn = *v.app.task.TaskArn
	case TaskDefinitionKind:
		arn = *v.app.taskDefinition.TaskDefinitionArn
	case ServiceDeploymentKind:
		arn = *v.app.serviceDeployment.ServiceDeploymentArn
	case LambdaKind:
		if selected.lambdaFunction != nil {
			url = utils.LambdaFunctionURL(region, awsToString(selected.lambdaFunction.FunctionName))
		}
	case SQSKind:
		url = utils.SQSQueueURL(region, selected.sqsQueueName)
	case SQSPeekKind:
		url = utils.SQSQueueURL(region, v.app.sqsQueueName)
	case DynamoDBKind:
		if selected.ddbTable != nil {
			url = utils.DynamoDBTableURL(region, awsToString(selected.ddbTable.TableName))
		}
	case DynamoDBIndexKind, DynamoDBScanKind:
		if v.app.ddbTable != nil {
			url = utils.DynamoDBTableURL(region, awsToString(v.app.ddbTable.TableName))
		}
	default:
		v.app.Notice.Warnf("open in browser not supported for %s", v.app.kind)
		return
	}
	if url == "" {
		url = utils.ArnToUrl(arn, taskService)
	}
	if len(url) == 0 {
		slog.Warn("open failed", "url", url, "kind", v.app.kind, "arn", arn)
		v.app.Notice.Warnf("open in browser not supported for %s", v.app.kind)
		return
	}
	slog.Info("open", "url", url)
	err = utils.OpenURL(url)
	if err != nil {
		v.app.Notice.Warnf("failed to open url %s\n", url)
	}
}

// awsToString is a tiny helper so openInBrowser doesn't need to import the AWS
// SDK just to dereference a *string. Mirrors aws.ToString without the dep.
func awsToString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
