package view

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
	"github.com/rivo/tview"
)

// s3Kind is the cross-process inventory cache for S3 buckets. Mirrors
// lambdaKind: implements kindpkg.Resource for the registry-driven
// dispatcher and exposes a loadInventory single-flight so the auto-refresh
// ticker and `:s3` palette entry share one network round-trip.
func init() {
	kindpkg.Register(&s3Kind{})
	bindKind(S3Kind, "s3", "s3", "buckets")
}

type s3Kind struct {
	kindpkg.BaseKind
	selected *s3Types.Bucket
	mu       sync.RWMutex
	buckets  []s3Types.Bucket
	loaded   bool
	loadDone chan struct{}
	loadErr  error
}

func (k *s3Kind) Name() string  { return "s3" }
func (k *s3Kind) Title() string { return "buckets" }

func (k *s3Kind) Aliases() []string { return []string{"buckets"} }

func (k *s3Kind) Show(host kindpkg.Host, reload bool) error {
	if app, ok := host.(*App); ok {
		return app.showS3Page(reload)
	}
	return nil
}

func (k *s3Kind) Reset() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.selected = nil
	k.buckets = nil
	k.loaded = false
	k.loadDone = nil
	k.loadErr = nil
}

func (k *s3Kind) Selection() any {
	if k.selected == nil {
		return nil
	}
	return k.selected
}

func (k *s3Kind) SetSelection(s any) {
	if b, ok := s.(*s3Types.Bucket); ok {
		k.selected = b
	}
}

// BrowserURL returns the AWS console URL for the bucket under the cursor.
// S3 is a global service but the console URL still carries a region for
// the host, so use the active region (or us-east-1 fallback in utils).
func (k *s3Kind) BrowserURL(region string) (string, error) {
	b := k.selected
	if b == nil || b.Name == nil {
		return "", nil
	}
	return utils.S3BucketURL(region, aws.ToString(b.Name)), nil
}

func (k *s3Kind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "buckets"}
}

// Traits opt the bucket list into filter, refresh, browser, and the wide-
// table arrow-scroll affordance other flat kinds use.
func (k *s3Kind) Traits() kindpkg.Traits {
	return kindpkg.Traits{
		Filterable:  true,
		Refreshable: true,
		Browsable:   true,
		WideTable:   true,
	}
}

func (k *s3Kind) Preload(app kindpkg.App) {
	_ = k.loadInventory(app, false)
}

// Refresh satisfies kindpkg.Refresher. Called off the tview event loop by
// the auto-refresh ticker so the AWS round-trip never blocks scroll input.
func (k *s3Kind) Refresh(app kindpkg.App) error {
	return k.loadInventory(app, true)
}

// loadInventory fetches the bucket list and caches the result. Concurrent
// callers single-flight on k.loadDone. When reload is true the cache is
// invalidated before the fetch so refresh keys (`r`) and the auto-refresh
// ticker actually re-hit S3; selection state is preserved across refresh.
func (k *s3Kind) loadInventory(app kindpkg.App, reload bool) error {
	k.mu.Lock()
	if reload {
		k.loaded = false
		k.buckets = nil
		k.loadErr = nil
		k.loadDone = nil
	}
	if k.loaded {
		k.mu.Unlock()
		return nil
	}
	if k.loadDone != nil {
		done := k.loadDone
		k.mu.Unlock()
		<-done
		k.mu.RLock()
		err := k.loadErr
		k.mu.RUnlock()
		return err
	}
	done := make(chan struct{})
	k.loadDone = done
	k.mu.Unlock()

	buckets, err := app.AWSClients().ListBuckets(context.Background())

	k.mu.Lock()
	k.loadErr = err
	if err == nil {
		k.buckets = buckets
		k.loaded = true
	} else {
		k.loadDone = nil
	}
	k.mu.Unlock()
	close(done)
	return err
}

// getS3Kind retrieves the registered s3Kind cache. Returns nil if the
// kind isn't in the registry (shouldn't happen given init() above, but the
// fallback keeps the page resilient to registry changes).
func getS3Kind() *s3Kind {
	k, ok := kindpkg.Get("s3")
	if !ok {
		return nil
	}
	sk, _ := k.(*s3Kind)
	return sk
}

// ---- Legacy-style ECS chrome integration ----

type s3View struct {
	view
	buckets []s3Types.Bucket
}

func newS3View(buckets []s3Types.Bucket, app *App) *s3View {
	return &s3View{
		view: *newView(app, basicKeyInputs, secondaryPageKeyMap{
			DescriptionKind: describePageKeys,
		}),
		buckets: buckets,
	}
}

// showS3Page is the S3Kind entry point reachable from showPrimaryKindPage.
// Uses the legacy buildResourcePage flow so chrome matches ECS pages exactly.
func (app *App) showS3Page(reload bool) error {
	app.kind = S3Kind
	if switched := app.switchPage(reload); switched {
		return nil
	}
	sk := getS3Kind()
	if sk != nil {
		if err := sk.loadInventory(app, reload); err != nil {
			return err
		}
		sk.mu.RLock()
		buckets := append([]s3Types.Bucket(nil), sk.buckets...)
		sk.mu.RUnlock()
		return buildResourcePage(buckets, app, nil, func() resourceViewBuilder {
			return newS3View(buckets, app)
		})
	}
	buckets, err := app.Clients.ListBuckets(context.Background())
	return buildResourcePage(buckets, app, err, func() resourceViewBuilder {
		return newS3View(buckets, app)
	})
}

func (v *s3View) getViewAndFooter() (*view, *tview.TextView) {
	return &v.view, v.footer.middle
}

func (v *s3View) headerParamsBuilder() []headerPageParam {
	params := make([]headerPageParam, 0, len(v.buckets))
	for i, b := range v.buckets {
		params = append(params, headerPageParam{
			title:      aws.ToString(b.Name),
			entityName: aws.ToString(b.Name),
			items:      v.headerPageItems(i),
		})
	}
	return params
}

func (v *s3View) headerPageItems(index int) []headerItem {
	b := v.buckets[index]
	return []headerItem{
		{name: "Name", value: aws.ToString(b.Name)},
		{name: "Created", value: utils.ShowTime(b.CreationDate)},
		{name: "Region", value: stringOrEmpty(b.BucketRegion)},
	}
}

func stringOrEmpty(s *string) string {
	if s == nil || *s == "" {
		return utils.EmptyText
	}
	return *s
}

func (v *s3View) tableParamsBuilder() (title string, headers []string, rowsBuilder func() [][]string) {
	title = fmt.Sprintf(color.TableTitleFmt, v.app.kind, "all", len(v.buckets))
	headers = []string{
		"Name",
		"Region",
		"Created",
		"Age",
	}
	rowsBuilder = func() (data [][]string) {
		for _, b := range v.buckets {
			row := []string{
				aws.ToString(b.Name),
				stringOrEmpty(b.BucketRegion),
				utils.ShowTime(b.CreationDate),
				utils.Age(b.CreationDate),
			}
			data = append(data, row)
			copyB := b
			entity := Entity{
				s3Bucket:   &copyB,
				entityName: aws.ToString(b.Name),
			}
			v.originalRowReferences = append(v.originalRowReferences, entity)
		}
		return data
	}
	return
}
