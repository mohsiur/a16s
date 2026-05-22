package view

import (
	"fmt"

	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	"github.com/rivo/tview"
)

// View footer struct. The four left-side fields (cluster/service/task/container)
// are always shown; one of them gets the "selected" cyan highlight via
// resource_view.go when the corresponding view is active. `middle` is a
// single shared TextView for the current-kind cell that sits to the right of
// the four ECS items — every other kind (lambda/sqs/dynamodb/profile/...)
// returns it from getViewAndFooter so resource_view.go can highlight it.
type footer struct {
	footerFlex *tview.Flex
	cluster    *tview.TextView
	service    *tview.TextView
	task       *tview.TextView
	container  *tview.TextView
	middle     *tview.TextView
}

func newFooter() *footer {
	footerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	footerFlex.SetBackgroundColor(color.Color(theme.BgColor))
	return &footer{
		footerFlex: footerFlex,
		cluster:    tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ClusterKind)),
		service:    tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ServiceKind)),
		task:       tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, TaskKind)),
		container:  tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ContainerKind)),
		middle:     tview.NewTextView().SetDynamicColors(true).SetTextAlign(L),
	}
}

// middleFooterLabel returns the label shown in the middle footer slot for
// the active kind. The four ECS chain kinds (cluster/service/task/container)
// deliberately return "" here because their label is already shown in the
// always-visible left row — historically the legacy if/else fell through to
// the empty-stretch branch for them.
//
// All other migrated kinds answer via kindpkg.Resource.FooterItem. The four
// unmigrated middle-slot kinds (profile/region/help/instance) still go
// through the enum's String() until they migrate.
func middleFooterLabel(k kind) string {
	switch k {
	case ClusterKind, ServiceKind, TaskKind, ContainerKind:
		return ""
	}
	if r := resolveResource(k); r != nil {
		if item := r.FooterItem(); item.Label != "" {
			return item.Label
		}
	}
	switch k {
	case ProfileKind, RegionKind, HelpKind, InstanceKind:
		return k.String()
	}
	return ""
}

func (v *view) addFooterItems() {
	// left resources
	v.footer.footerFlex.AddItem(v.footer.cluster, 13, 0, false).
		AddItem(v.footer.service, 13, 0, false).
		AddItem(v.footer.task, 10, 0, false).
		AddItem(v.footer.container, 15, 0, false)

	// middle "current kind" cell. resource_view.go has already SetText-ed the
	// selected-format label onto v.footer.middle for migrated/unmigrated
	// kinds; for the four ECS chain kinds the middle slot is unused (their
	// label lives in the always-shown left row) and we fill the gap with an
	// empty flex.
	if middleFooterLabel(v.app.kind) != "" {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.middle, 0, 1, false)
	} else {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 0, 1, false)
	}

	// aws profile and region
	awsInfo := fmt.Sprintf("%s:%s", globalProfile, globalRegion)
	awsInfoView := tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterAwsFmt, awsInfo))
	v.footer.footerFlex.AddItem(awsInfoView, len(awsInfo)+3, 0, false)

	// app info label
	t := tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterAppFmt, utils.AppName, utils.AppVersion))
	v.footer.footerFlex.AddItem(t, len(utils.AppVersion)+7, 0, false)
}
