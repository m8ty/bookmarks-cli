package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rivo/tview"
)

type Bookmarks map[string][]string

const (
	sourceHistory = iota
	sourceBookmarks
)

var bookmarksFile = filepath.Join(os.Getenv("HOME"), ".config", "bookmarks", "saved.json")

func loadHistory() []string {
	out, _ := exec.Command("bash", "-c", "cat ~/.zsh_history | cut -d ';' -f 2- | tail -n 500").Output()
	lines := strings.Split(string(out), "\n")
	// Most-recent-first, deduped.
	items := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		items = append(items, l)
	}
	return items
}

func loadBookmarks() Bookmarks {
	data, err := os.ReadFile(bookmarksFile)
	if err != nil {
		return Bookmarks{}
	}
	var b Bookmarks
	if err := json.Unmarshal(data, &b); err != nil || b == nil {
		return Bookmarks{}
	}
	return b
}

func saveBookmarks(b Bookmarks) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bookmarksFile, data, 0644)
}

func main() {
	os.MkdirAll(filepath.Dir(bookmarksFile), 0755)

	cwd, _ := os.Getwd()
	history := loadHistory()
	bookmarks := loadBookmarks()

	app := tview.NewApplication()
	var commandToRun string

	source := sourceHistory
	sourceName := func() string {
		if source == sourceHistory {
			return "history"
		}
		return "bookmarks"
	}
	currentItems := func() []string {
		if source == sourceHistory {
			return history
		}
		return bookmarks[cwd]
	}

	input := tview.NewInputField()
	input.SetLabel(" › ")
	input.SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetLabelColor(tcell.ColorMediumPurple)

	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetHighlightFullLine(true)
	list.SetWrapAround(true)
	list.SetSelectedBackgroundColor(tcell.ColorMediumPurple)
	list.SetSelectedTextColor(tcell.ColorWhite)
	list.SetMainTextColor(tcell.ColorDefault)
	list.SetBorderPadding(0, 0, 1, 1)

	const helpText = "  ↵ run   ⌃B bookmark   ⌃D delete   ⌃T toggle source   esc quit"
	footer := tview.NewTextView()
	footer.SetText(helpText)
	footer.SetTextColor(tcell.ColorGray)
	footer.SetDynamicColors(true)

	panel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(input, 1, 0, true).
		AddItem(list, 0, 1, false)
	panel.SetBorder(true)
	panel.SetBorderColor(tcell.ColorMediumPurple)
	panel.SetTitleAlign(tview.AlignLeft)

	updateTitle := func() {
		panel.SetTitle(fmt.Sprintf(" cli-bookmark · %s (%d) ", sourceName(), len(currentItems())))
	}

	refreshList := func() {
		items := currentItems()
		text := strings.TrimSpace(input.GetText())
		matches := items
		if text != "" {
			matches = fuzzy.Find(text, items)
		}
		list.Clear()
		for _, m := range matches {
			list.AddItem(m, "", 0, nil)
		}
		updateTitle()
	}
	refreshList()

	flashStatus := func(msg string) {
		footer.SetText("  [yellow]" + tview.Escape(msg) + "[-]")
		go func() {
			time.Sleep(1500 * time.Millisecond)
			app.QueueUpdateDraw(func() {
				footer.SetText(helpText)
			})
		}()
	}

	selected := func() string {
		if list.GetItemCount() == 0 {
			return strings.TrimSpace(input.GetText())
		}
		text, _ := list.GetItemText(list.GetCurrentItem())
		return text
	}

	bookmarkCurrent := func() {
		cmd := selected()
		if cmd == "" {
			return
		}
		for _, existing := range bookmarks[cwd] {
			if existing == cmd {
				flashStatus("Already bookmarked")
				return
			}
		}
		bookmarks[cwd] = append(bookmarks[cwd], cmd)
		if err := saveBookmarks(bookmarks); err != nil {
			flashStatus("Save error: " + err.Error())
			return
		}
		flashStatus("Bookmarked: " + cmd)
		if source == sourceBookmarks {
			refreshList()
		}
	}

	deleteBookmark := func() {
		if source != sourceBookmarks {
			flashStatus("Switch to bookmarks (⌃T) to delete")
			return
		}
		cmd := selected()
		if cmd == "" {
			return
		}
		items := bookmarks[cwd]
		out := items[:0]
		removed := false
		for _, c := range items {
			if c == cmd && !removed {
				removed = true
				continue
			}
			out = append(out, c)
		}
		if !removed {
			flashStatus("Not in bookmarks")
			return
		}
		bookmarks[cwd] = out
		if err := saveBookmarks(bookmarks); err != nil {
			flashStatus("Save error: " + err.Error())
			return
		}
		flashStatus("Deleted: " + cmd)
		refreshList()
	}

	toggleSource := func() {
		if source == sourceHistory {
			source = sourceBookmarks
		} else {
			source = sourceHistory
		}
		refreshList()
	}

	input.SetChangedFunc(func(_ string) {
		refreshList()
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyDown, tcell.KeyCtrlJ:
			n := list.GetItemCount()
			if n > 0 {
				list.SetCurrentItem((list.GetCurrentItem() + 1) % n)
			}
			return nil
		case tcell.KeyUp, tcell.KeyCtrlK:
			n := list.GetItemCount()
			if n > 0 {
				cur := list.GetCurrentItem() - 1
				if cur < 0 {
					cur = n - 1
				}
				list.SetCurrentItem(cur)
			}
			return nil
		case tcell.KeyCtrlT:
			toggleSource()
			return nil
		case tcell.KeyCtrlB:
			bookmarkCurrent()
			return nil
		case tcell.KeyCtrlD:
			deleteBookmark()
			return nil
		case tcell.KeyEsc:
			app.Stop()
			return nil
		}
		return event
	})

	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		commandToRun = selected()
		app.Stop()
	})

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(panel, 0, 1, true).
		AddItem(footer, 1, 0, false)

	if err := app.SetRoot(root, true).Run(); err != nil {
		panic(err)
	}

	if commandToRun == "" {
		return
	}

	copyCmd := exec.Command("pbcopy")
	copyCmd.Stdin = strings.NewReader(commandToRun)
	if err := copyCmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "pbcopy:", err)
	}

	cmd := exec.Command("bash", "-c", commandToRun)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "command failed:", err)
	}
}
