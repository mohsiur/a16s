package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// noopView is the kind.View returned by adapter kinds that drive the existing
// pages stack themselves rather than rendering a new tview.Flex. Used by ECS
// adapters in Phase 5.
type noopView struct{}

func (n *noopView) Render() *tview.Flex                        { return tview.NewFlex() }
func (n *noopView) Focus()                                     {}
func (n *noopView) OnKey(event *tcell.EventKey) (handled bool) { return false }
