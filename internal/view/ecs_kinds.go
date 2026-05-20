package view

import (
	"errors"

	kindpkg "github.com/mohsiur/a16s/internal/view/kind"
)

// ECS adapter kinds bridge the new palette to the legacy ECS pages stack.
// Typing `:cluster`, `:service`, or `:task` lands on the existing drill-down
// views by calling showPrimaryKindPage and returning a *noopView so
// App.SwitchView skips swapping the main page (see app.go SwitchView).

func init() {
	kindpkg.Register(&ecsClusterKind{})
	kindpkg.Register(&ecsServiceKind{})
	kindpkg.Register(&ecsTaskKind{})
}

type ecsClusterKind struct{}

func (e *ecsClusterKind) Name() string                        { return "cluster" }
func (e *ecsClusterKind) Reset()                              {}
func (e *ecsClusterKind) Selection() any                      { return nil }
func (e *ecsClusterKind) SetSelection(any)                    {}
func (e *ecsClusterKind) Breadcrumb() string                  { return "cluster" }
func (e *ecsClusterKind) PrimaryAction() kindpkg.Action       { return nil }
func (e *ecsClusterKind) SecondaryActions() []kindpkg.Binding { return nil }
func (e *ecsClusterKind) Build(app kindpkg.App) (kindpkg.View, error) {
	concrete, ok := app.(*App)
	if !ok {
		return nil, errors.New("ecsClusterKind needs *view.App")
	}
	if err := concrete.showPrimaryKindPage(ClusterKind, true); err != nil {
		return nil, err
	}
	return &noopView{}, nil
}

type ecsServiceKind struct{}

func (e *ecsServiceKind) Name() string                        { return "service" }
func (e *ecsServiceKind) Reset()                              {}
func (e *ecsServiceKind) Selection() any                      { return nil }
func (e *ecsServiceKind) SetSelection(any)                    {}
func (e *ecsServiceKind) Breadcrumb() string                  { return "service" }
func (e *ecsServiceKind) PrimaryAction() kindpkg.Action       { return nil }
func (e *ecsServiceKind) SecondaryActions() []kindpkg.Binding { return nil }
func (e *ecsServiceKind) Build(app kindpkg.App) (kindpkg.View, error) {
	concrete, ok := app.(*App)
	if !ok {
		return nil, errors.New("ecsServiceKind needs *view.App")
	}
	if err := concrete.showPrimaryKindPage(ServiceKind, true); err != nil {
		return nil, err
	}
	return &noopView{}, nil
}

type ecsTaskKind struct{}

func (e *ecsTaskKind) Name() string                        { return "task" }
func (e *ecsTaskKind) Reset()                              {}
func (e *ecsTaskKind) Selection() any                      { return nil }
func (e *ecsTaskKind) SetSelection(any)                    {}
func (e *ecsTaskKind) Breadcrumb() string                  { return "task" }
func (e *ecsTaskKind) PrimaryAction() kindpkg.Action       { return nil }
func (e *ecsTaskKind) SecondaryActions() []kindpkg.Binding { return nil }
func (e *ecsTaskKind) Build(app kindpkg.App) (kindpkg.View, error) {
	concrete, ok := app.(*App)
	if !ok {
		return nil, errors.New("ecsTaskKind needs *view.App")
	}
	if err := concrete.showPrimaryKindPage(TaskKind, true); err != nil {
		return nil, err
	}
	return &noopView{}, nil
}
