package view

import (
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/mohsiur/a16s/internal/utils"
	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// ECS chain Resource adapters. Unlike Lambda, the ECS kinds (cluster, service,
// task, container, task definition, service deployment) are not registered as
// inventory caches — their list pages are still owned by the legacy `view`
// package (cluster.go, service.go, task.go, ...). These kinds exist purely as
// a routing target for kindpkg.Resource.BrowserURL so `o` on an ECS row goes
// through the same dispatcher as Lambda. Each one captures whatever selection
// state ArnToUrl needs (and, for task/container, the parent service name),
// updated via SetSelection in changeSelectedValues.
func init() {
	kindpkg.Register(&clusterKind{})
	kindpkg.Register(&serviceKind{})
	kindpkg.Register(&taskKind{})
	kindpkg.Register(&containerKind{})
	kindpkg.Register(&taskDefinitionKind{})
	kindpkg.Register(&serviceDeploymentKind{})

	bindKind(ClusterKind, "clusters", "clusters")
	bindKind(ServiceKind, "services")
	bindKind(TaskKind, "tasks")
	bindKind(ContainerKind, "containers")
	bindKind(TaskDefinitionKind, "task-definitions")
	bindKind(ServiceDeploymentKind, "service-deployments")
}

// clusterKind adapts ECS cluster selection to kindpkg.Resource for browser
// dispatch. The list page itself remains in cluster.go.
type clusterKind struct {
	kindpkg.BaseKind
	selected *types.Cluster
}

func (k *clusterKind) Name() string     { return "clusters" }
func (k *clusterKind) Reset()           { k.selected = nil }
func (k *clusterKind) Selection() any   { return k.selected }
func (k *clusterKind) SetSelection(s any) {
	if c, ok := s.(*types.Cluster); ok {
		k.selected = c
	}
}
func (k *clusterKind) BrowserURL(_ string) (string, error) {
	if k.selected == nil || k.selected.ClusterArn == nil {
		return "", nil
	}
	return utils.ArnToUrl(*k.selected.ClusterArn, ""), nil
}
func (k *clusterKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "clusters"}
}

func getClusterKind() *clusterKind {
	k, ok := kindpkg.Get("clusters")
	if !ok {
		return nil
	}
	ck, _ := k.(*clusterKind)
	return ck
}

// serviceKind adapts ECS service selection to kindpkg.Resource for browser
// dispatch. The selected service's name is also read by taskKind/containerKind
// at BrowserURL time since ArnToUrl needs it for task/container ARNs which
// don't carry the service segment.
type serviceKind struct {
	kindpkg.BaseKind
	selected *types.Service
}

func (k *serviceKind) Name() string     { return "services" }
func (k *serviceKind) Reset()           { k.selected = nil }
func (k *serviceKind) Selection() any   { return k.selected }
func (k *serviceKind) SetSelection(s any) {
	if svc, ok := s.(*types.Service); ok {
		k.selected = svc
	}
}
func (k *serviceKind) BrowserURL(_ string) (string, error) {
	if k.selected == nil || k.selected.ServiceArn == nil {
		return "", nil
	}
	return utils.ArnToUrl(*k.selected.ServiceArn, ""), nil
}
func (k *serviceKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "services"}
}

// PageHandle returns the parent cluster's ARN so service pages stay scoped
// to the active cluster.
func (k *serviceKind) PageHandle() string {
	if ck := getClusterKind(); ck != nil && ck.selected != nil && ck.selected.ClusterArn != nil {
		return *ck.selected.ClusterArn
	}
	return ""
}

func getServiceKind() *serviceKind {
	k, ok := kindpkg.Get("services")
	if !ok {
		return nil
	}
	sk, _ := k.(*serviceKind)
	return sk
}

// taskKind adapts ECS task selection to kindpkg.Resource for browser dispatch.
// ArnToUrl needs the parent service name for task ARNs; that's read from the
// active serviceKind selection rather than duplicated here so a single source
// of truth (the service registry entry) drives both kinds.
type taskKind struct {
	kindpkg.BaseKind
	selected *types.Task
}

func (k *taskKind) Name() string     { return "tasks" }
func (k *taskKind) Reset()           { k.selected = nil }
func (k *taskKind) Selection() any   { return k.selected }
func (k *taskKind) SetSelection(s any) {
	if t, ok := s.(*types.Task); ok {
		k.selected = t
	}
}
func (k *taskKind) BrowserURL(_ string) (string, error) {
	if k.selected == nil || k.selected.TaskArn == nil {
		return "", nil
	}
	svcName := ""
	if sk := getServiceKind(); sk != nil && sk.selected != nil && sk.selected.ServiceName != nil {
		svcName = *sk.selected.ServiceName
	}
	if svcName == "" {
		// Parent service hasn't been mirrored yet (e.g. an entry path that
		// bypassed changeSelectedValues). Caller falls back to reading the
		// parent service directly via openInBrowser.
		return "", nil
	}
	return utils.ArnToUrl(*k.selected.TaskArn, svcName), nil
}
func (k *taskKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "tasks"}
}

// PageHandle returns the parent service's ARN so task pages stay scoped to
// the active service.
func (k *taskKind) PageHandle() string {
	if sk := getServiceKind(); sk != nil && sk.selected != nil && sk.selected.ServiceArn != nil {
		return *sk.selected.ServiceArn
	}
	return ""
}

func getTaskKind() *taskKind {
	k, ok := kindpkg.Get("tasks")
	if !ok {
		return nil
	}
	tk, _ := k.(*taskKind)
	return tk
}

// containerKind adapts ECS container selection. Note that the legacy browser
// path for ContainerKind builds the URL from the *parent task's* ARN (plus
// service name), not the container's own ARN — ArnToUrl maps both "task" and
// "container" ARNs to the same console page, but the legacy code happens to
// pass the task ARN. We mirror that exactly.
type containerKind struct {
	kindpkg.BaseKind
	selected *types.Container
}

func (k *containerKind) Name() string     { return "containers" }
func (k *containerKind) Reset()           { k.selected = nil }
func (k *containerKind) Selection() any   { return k.selected }
func (k *containerKind) SetSelection(s any) {
	if c, ok := s.(*types.Container); ok {
		k.selected = c
	}
}
func (k *containerKind) BrowserURL(_ string) (string, error) {
	tk := getTaskKind()
	if tk == nil || tk.selected == nil || tk.selected.TaskArn == nil {
		return "", nil
	}
	svcName := ""
	if sk := getServiceKind(); sk != nil && sk.selected != nil && sk.selected.ServiceName != nil {
		svcName = *sk.selected.ServiceName
	}
	if svcName == "" {
		return "", nil
	}
	return utils.ArnToUrl(*tk.selected.TaskArn, svcName), nil
}
func (k *containerKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "containers"}
}

// PageHandle returns the parent task's ARN so container pages stay scoped
// to the active task.
func (k *containerKind) PageHandle() string {
	if tk := getTaskKind(); tk != nil && tk.selected != nil && tk.selected.TaskArn != nil {
		return *tk.selected.TaskArn
	}
	return ""
}

func getContainerKind() *containerKind {
	k, ok := kindpkg.Get("containers")
	if !ok {
		return nil
	}
	ck, _ := k.(*containerKind)
	return ck
}

// taskDefinitionKind adapts ECS task definition selection.
type taskDefinitionKind struct {
	kindpkg.BaseKind
	selected *types.TaskDefinition
}

func (k *taskDefinitionKind) Name() string     { return "task-definitions" }
func (k *taskDefinitionKind) Reset()           { k.selected = nil }
func (k *taskDefinitionKind) Selection() any   { return k.selected }
func (k *taskDefinitionKind) SetSelection(s any) {
	if td, ok := s.(*types.TaskDefinition); ok {
		k.selected = td
	}
}
func (k *taskDefinitionKind) BrowserURL(_ string) (string, error) {
	if k.selected == nil || k.selected.TaskDefinitionArn == nil {
		return "", nil
	}
	return utils.ArnToUrl(*k.selected.TaskDefinitionArn, ""), nil
}
func (k *taskDefinitionKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "task definitions"}
}

// PageHandle returns the parent service's ARN so task-definition pages stay
// scoped to the active service.
func (k *taskDefinitionKind) PageHandle() string {
	if sk := getServiceKind(); sk != nil && sk.selected != nil && sk.selected.ServiceArn != nil {
		return *sk.selected.ServiceArn
	}
	return ""
}

func getTaskDefinitionKind() *taskDefinitionKind {
	k, ok := kindpkg.Get("task-definitions")
	if !ok {
		return nil
	}
	tdk, _ := k.(*taskDefinitionKind)
	return tdk
}

// serviceDeploymentKind adapts ECS service deployment selection.
type serviceDeploymentKind struct {
	kindpkg.BaseKind
	selected *types.ServiceDeployment
}

func (k *serviceDeploymentKind) Name() string     { return "service-deployments" }
func (k *serviceDeploymentKind) Reset()           { k.selected = nil }
func (k *serviceDeploymentKind) Selection() any   { return k.selected }
func (k *serviceDeploymentKind) SetSelection(s any) {
	if sd, ok := s.(*types.ServiceDeployment); ok {
		k.selected = sd
	}
}
func (k *serviceDeploymentKind) BrowserURL(_ string) (string, error) {
	if k.selected == nil || k.selected.ServiceDeploymentArn == nil {
		return "", nil
	}
	return utils.ArnToUrl(*k.selected.ServiceDeploymentArn, ""), nil
}
func (k *serviceDeploymentKind) FooterItem() kindpkg.FooterItem {
	return kindpkg.FooterItem{Label: "service deployments"}
}

// PageHandle returns the parent service's ARN so service-deployment pages
// stay scoped to the active service.
func (k *serviceDeploymentKind) PageHandle() string {
	if sk := getServiceKind(); sk != nil && sk.selected != nil && sk.selected.ServiceArn != nil {
		return *sk.selected.ServiceArn
	}
	return ""
}

func getServiceDeploymentKind() *serviceDeploymentKind {
	k, ok := kindpkg.Get("service-deployments")
	if !ok {
		return nil
	}
	sdk, _ := k.(*serviceDeploymentKind)
	return sdk
}
