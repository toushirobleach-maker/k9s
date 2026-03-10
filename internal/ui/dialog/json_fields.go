// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dialog

import (
	"fmt"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/ui"
	"github.com/derailed/tcell/v2"
	"github.com/derailed/tview"
)

const jsonFieldsKey = "jsonFields"
const jsonFieldsHelp = " <Space> Toggle  <a> Select All  <d> Deselect All  <o> OK  <c> Cancel "

type jsonFieldsModal struct {
	*tview.Box
	frame       *tview.Frame
	list        *tview.List
	form        *tview.Form
	btnCount    int
	contentSize int
	maxItemLen  int
}

func newJSONFieldsModal(title string, list *tview.List, buttons *tview.Form, maxItemLen int, contentSize int) *jsonFieldsModal {
	m := &jsonFieldsModal{Box: tview.NewBox()}
	m.list = list
	m.form = buttons
	m.btnCount = buttons.GetButtonCount()
	m.maxItemLen = maxItemLen
	m.contentSize = contentSize

	content := tview.NewFlex().SetDirection(tview.FlexRow)
	content.AddItem(list, 0, 1, true)
	content.AddItem(buttons, 4, 1, false)

	m.frame = tview.NewFrame(content).SetBorders(0, 0, 1, 0, 0, 0)
	m.frame.SetBorder(true).SetTitle("<" + title + ">")

	return m
}

func (m *jsonFieldsModal) Draw(screen tcell.Screen) {
	screenWidth, screenHeight := screen.Size()
	width := min(max(m.maxItemLen+10, len(jsonFieldsHelp)+4, 64), screenWidth-4)
	height := min(m.contentSize, screenHeight-4)
	if width < 20 {
		width = 20
	}
	if height < 8 {
		height = 8
	}
	x := (screenWidth - width) / 2
	y := (screenHeight - height) / 2
	m.SetRect(x, y, width, height)
	m.frame.SetRect(x, y, width, height)
	m.frame.Draw(screen)
}

func (m *jsonFieldsModal) Focus(delegate func(p tview.Primitive)) {
	if m.form.HasFocus() {
		delegate(m.form)
		return
	}
	delegate(m.list)
}

func (m *jsonFieldsModal) HasFocus() bool {
	return m.list.HasFocus() || m.form.HasFocus()
}

func (m *jsonFieldsModal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return m.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		consumed, capture = m.frame.MouseHandler()(action, event, setFocus)
		if !consumed && action == tview.MouseLeftClick && m.InRect(event.Position()) {
			setFocus(m)
			consumed = true
		}
		return
	})
}

func (m *jsonFieldsModal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if m.list.HasFocus() && event.Key() == tcell.KeyDown {
			if m.list.GetCurrentItem() == m.list.GetItemCount()-1 {
				if m.btnCount > 0 {
					m.form.SetFocus(0)
				}
				setFocus(m.form)
				return
			}
		}
		if m.list.HasFocus() && event.Key() == tcell.KeyUp {
			if m.list.GetCurrentItem() == 0 {
				if m.btnCount > 0 {
					m.form.SetFocus(m.btnCount - 1)
				}
				setFocus(m.form)
				return
			}
		}
		if m.form.HasFocus() && event.Key() == tcell.KeyUp {
			_, btn := m.form.GetFocusedItemIndex()
			if btn == 0 {
				setFocus(m.list)
				m.list.SetCurrentItem(m.list.GetItemCount() - 1)
				return
			}
		}
		if m.form.HasFocus() && event.Key() == tcell.KeyDown {
			_, btn := m.form.GetFocusedItemIndex()
			if btn == m.btnCount-1 {
				setFocus(m.list)
				m.list.SetCurrentItem(0)
				return
			}
		}
		switch {
		case m.list.HasFocus():
			if handler := m.list.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
		case m.form.HasFocus():
			if handler := m.form.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
		default:
			if handler := m.frame.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
		}
	})
}

func ShowJSONFields(styles *config.Dialog, pages *ui.Pages, title string, fields []string, selected map[string]struct{}, allSelected bool, ok func(all bool, selected map[string]struct{}), cancel cancelFunc) {
	if len(fields) == 0 {
		cancel()
		return
	}

	selection := make(map[string]bool, len(fields))
	if allSelected {
		for _, f := range fields {
			selection[f] = true
		}
	} else {
		for k := range selected {
			selection[k] = true
		}
	}

	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetSelectedTextColor(styles.ButtonFocusFgColor.Color())
	list.SetSelectedBackgroundColor(styles.ButtonFocusBgColor.Color())
	list.SetMainTextColor(styles.FieldFgColor.Color())
	list.SetBackgroundColor(styles.BgColor.Color())
	list.SetBorder(true)
	list.SetTitle(jsonFieldsHelp)

	maxItemLen := 0
	for _, f := range fields {
		txt := itemText(f, selection[f])
		list.AddItem(txt, "", 0, nil)
		if l := len(f) + 4; l > maxItemLen {
			maxItemLen = l
		}
	}

	toggle := func(i int) {
		if i < 0 || i >= len(fields) {
			return
		}
		key := fields[i]
		selection[key] = !selection[key]
		list.SetItemText(i, itemText(key, selection[key]), "")
	}

	applyAll := func(on bool) {
		for i, f := range fields {
			selection[f] = on
			list.SetItemText(i, itemText(f, on), "")
		}
	}

	applyOK := func() {
		out := make(map[string]struct{})
		count := 0
		for _, f := range fields {
			if selection[f] {
				out[f] = struct{}{}
				count++
			}
		}
		all := count == 0 || count == len(fields)
		ok(all, out)
		dismissJSONFields(pages)
		cancel()
	}

	list.SetInputCapture(func(evt *tcell.EventKey) *tcell.EventKey {
		switch evt.Key() {
		case tcell.KeyRune:
			switch evt.Rune() {
			case ' ':
				toggle(list.GetCurrentItem())
				return nil
			case 'a', 'A':
				applyAll(true)
				return nil
			case 'd', 'D':
				applyAll(false)
				return nil
			case 'o', 'O':
				applyOK()
				return nil
			case 'c', 'C':
				dismissJSONFields(pages)
				cancel()
				return nil
			}
		case tcell.KeyEnter:
			applyOK()
			return nil
		case tcell.KeyEsc:
			dismissJSONFields(pages)
			cancel()
			return nil
		}
		return evt
	})
	list.SetSelectedFunc(func(i int, _ string, _ string, _ rune) {
		toggle(i)
	})

	form := tview.NewForm()
	form.SetItemPadding(0)
	form.SetButtonsAlign(tview.AlignCenter).
		SetButtonBackgroundColor(styles.ButtonBgColor.Color()).
		SetButtonTextColor(styles.ButtonFgColor.Color())

	form.AddButton("OK", applyOK)
	form.AddButton("Select All", func() { applyAll(true) })
	form.AddButton("Deselect All", func() { applyAll(false) })
	form.AddButton("Cancel", func() {
		dismissJSONFields(pages)
		cancel()
	})
	form.SetInputCapture(func(evt *tcell.EventKey) *tcell.EventKey {
		if evt.Key() == tcell.KeyRune {
			switch evt.Rune() {
			case 'o', 'O':
				applyOK()
				return nil
			case 'a', 'A':
				applyAll(true)
				return nil
			case 'd', 'D':
				applyAll(false)
				return nil
			case 'c', 'C':
				dismissJSONFields(pages)
				cancel()
				return nil
			}
		}
		return evt
	})

	buttonCount := form.GetButtonCount()
	for i := range buttonCount {
		if b := form.GetButton(i); b != nil {
			b.SetBackgroundColorActivated(styles.ButtonFocusBgColor.Color())
			b.SetLabelColorActivated(styles.ButtonFocusFgColor.Color())
		}
	}

	contentSize := len(fields) + 8
	modal := newJSONFieldsModal(title, list, form, maxItemLen, contentSize)
	modal.frame.SetTitleColor(styles.FgColor.Color())

	pages.AddPage(jsonFieldsKey, modal, false, false)
	pages.ShowPage(jsonFieldsKey)
}

func dismissJSONFields(pages *ui.Pages) {
	pages.RemovePage(jsonFieldsKey)
}

func itemText(field string, selected bool) string {
	if selected {
		return fmt.Sprintf("[x] %s", field)
	}
	return fmt.Sprintf("[ ] %s", field)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(vv ...int) int {
	m := vv[0]
	for _, v := range vv[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
