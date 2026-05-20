package view

import (
	"github.com/gdamore/tcell/v2"
	kindpkg "github.com/keidarcy/e1s/internal/view/kind"
	"github.com/rivo/tview"
)

// simpleKindView is the default kind.View for table-based flat kinds.
type simpleKindView struct {
	flex   *tview.Flex
	focus  tview.Primitive
	app    kindpkg.App
	source kindpkg.Kind
}

func (s *simpleKindView) Render() *tview.Flex { return s.flex }
func (s *simpleKindView) Focus()              { /* tview handles focus via SetFocus */ }
func (s *simpleKindView) OnKey(event *tcell.EventKey) (handled bool) {
	if s.source == nil {
		return false
	}
	if event.Key() == tcell.KeyEnter {
		if act := s.source.PrimaryAction(); act != nil {
			act(s.app)
			return true
		}
	}
	for _, b := range s.source.SecondaryActions() {
		if event.Rune() == b.Key && b.Run != nil {
			b.Run(s.app)
			return true
		}
	}
	return false
}
