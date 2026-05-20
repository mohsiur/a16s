package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// simpleKindView is the default kind.View for table-based flat kinds.
type simpleKindView struct {
	flex  *tview.Flex
	focus tview.Primitive
}

func (s *simpleKindView) Render() *tview.Flex                        { return s.flex }
func (s *simpleKindView) Focus()                                     { /* tview handles focus via SetFocus */ }
func (s *simpleKindView) OnKey(event *tcell.EventKey) (handled bool) { return false }
