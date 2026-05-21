package view

import (
	"fmt"

	"github.com/mohsiur/a16s/internal/color"
	"github.com/mohsiur/a16s/internal/utils"
	"github.com/rivo/tview"
)

// View footer struct
type footer struct {
	footerFlex        *tview.Flex
	cluster           *tview.TextView
	service           *tview.TextView
	task              *tview.TextView
	container         *tview.TextView
	profile           *tview.TextView
	region            *tview.TextView
	instance          *tview.TextView
	taskDefinition    *tview.TextView
	serviceDeployment *tview.TextView
	help              *tview.TextView
	lambda            *tview.TextView
	sqs               *tview.TextView
	sqsPeek           *tview.TextView
	dynamodb          *tview.TextView
	ddbIndex          *tview.TextView
	ddbScan           *tview.TextView
}

func newFooter() *footer {
	footerFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	footerFlex.SetBackgroundColor(color.Color(theme.BgColor))
	return &footer{
		footerFlex:        footerFlex,
		cluster:           tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ClusterKind)),
		service:           tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ServiceKind)),
		task:              tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, TaskKind)),
		container:         tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ContainerKind)),
		profile:           tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ProfileKind)),
		region:            tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, RegionKind)),
		instance:          tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, InstanceKind)).SetTextAlign(L),
		taskDefinition:    tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, TaskDefinitionKind)).SetTextAlign(L),
		serviceDeployment: tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, ServiceDeploymentKind)).SetTextAlign(L),
		help:              tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, HelpKind)).SetTextAlign(L),
		lambda:            tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, LambdaKind)).SetTextAlign(L),
		sqs:               tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, SQSKind)).SetTextAlign(L),
		sqsPeek:           tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, SQSPeekKind)).SetTextAlign(L),
		dynamodb:          tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, DynamoDBKind)).SetTextAlign(L),
		ddbIndex:          tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, DynamoDBIndexKind)).SetTextAlign(L),
		ddbScan:           tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf(color.FooterItemFmt, DynamoDBScanKind)).SetTextAlign(L),
	}
}
func (v *view) addFooterItems() {
	// left resources
	v.footer.footerFlex.AddItem(v.footer.cluster, 13, 0, false).
		AddItem(v.footer.service, 13, 0, false).
		AddItem(v.footer.task, 10, 0, false).
		AddItem(v.footer.container, 15, 0, false)

	// keep middle space
	if v.app.kind == TaskDefinitionKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.taskDefinition, 0, 1, false)
	} else if v.app.kind == InstanceKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.instance, 0, 1, false)
	} else if v.app.kind == ServiceDeploymentKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.serviceDeployment, 0, 1, false)
	} else if v.app.kind == HelpKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.help, 0, 1, false)
	} else if v.app.kind == ProfileKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.profile, 0, 1, false)
	} else if v.app.kind == RegionKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.region, 0, 1, false)
	} else if v.app.kind == LambdaKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.lambda, 0, 1, false)
	} else if v.app.kind == SQSKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.sqs, 0, 1, false)
	} else if v.app.kind == SQSPeekKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.sqsPeek, 0, 1, false)
	} else if v.app.kind == DynamoDBKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.dynamodb, 0, 1, false)
	} else if v.app.kind == DynamoDBIndexKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.ddbIndex, 0, 1, false)
	} else if v.app.kind == DynamoDBScanKind {
		v.footer.footerFlex.
			AddItem(tview.NewTextView(), 5, 0, false).
			AddItem(v.footer.ddbScan, 0, 1, false)
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
