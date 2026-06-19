package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rivo/tview"
)

// Bookmark is a saved command with an optional short label.
type Bookmark struct {
	Label   string `json:"label,omitempty"`
	Command string `json:"command"`
}

// UnmarshalJSON accepts either the old format (a plain command string) or the
// new object form, so existing saved.json files keep working.
func (b *Bookmark) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		b.Command = s
		return nil
	}
	type raw Bookmark
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*b = Bookmark(r)
	return nil
}

type Bookmarks map[string][]Bookmark

const (
	sourceHistory = iota
	sourceBookmarks
)

var bookmarksFile = filepath.Join(os.Getenv("HOME"), ".config", "bookmarks", "saved.json")

// recencyWeight multiplies a command's frequency based on how long ago it was
// last run, so a command from minutes ago outranks a stale one. Modeled on the
// fasd/z "frecency" buckets.
func recencyWeight(ageSeconds int64) float64 {
	switch {
	case ageSeconds < 3600: // within the last hour
		return 4
	case ageSeconds < 86400: // within the last day
		return 2
	case ageSeconds < 604800: // within the last week
		return 0.5
	default:
		return 0.25
	}
}

// loadHistory reads recent zsh history and ranks it by frecency — frequency
// scaled by how recently each command was last run — so both your most-used and
// most-recent commands rise to the top.
//
// zsh extended history lines look like ": <epoch>:<elapsed>;<command>". Plain
// lines (no metadata prefix) are still accepted, just without a timestamp.
func loadHistory() []string {
	out, _ := exec.Command("bash", "-c", "cat ~/.zsh_history | tail -n 3000").Output()
	now := time.Now().Unix()

	type stat struct {
		count int
		last  int64 // epoch seconds of most-recent run; 0 if unknown
	}
	stats := map[string]*stat{}
	order := make([]string, 0) // first-seen order, for stable tie-breaking

	for _, raw := range strings.Split(string(out), "\n") {
		line := strings.TrimRight(raw, "\r")
		var ts int64
		cmd := line
		if strings.HasPrefix(line, ": ") {
			if semi := strings.IndexByte(line, ';'); semi >= 0 {
				meta := line[2:semi] // "<epoch>:<elapsed>"
				cmd = line[semi+1:]
				if colon := strings.IndexByte(meta, ':'); colon >= 0 {
					ts, _ = strconv.ParseInt(meta[:colon], 10, 64)
				}
			}
		}
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		s := stats[cmd]
		if s == nil {
			s = &stat{}
			stats[cmd] = s
			order = append(order, cmd)
		}
		s.count++
		if ts > s.last {
			s.last = ts
		}
	}

	type scored struct {
		cmd   string
		score float64
		last  int64
	}
	items := make([]scored, 0, len(stats))
	for _, cmd := range order {
		s := stats[cmd]
		age := now - s.last
		if s.last == 0 {
			age = 1 << 40 // unknown timestamp -> treat as ancient
		}
		items = append(items, scored{cmd, float64(s.count) * recencyWeight(age), s.last})
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].score != items[b].score {
			return items[a].score > items[b].score
		}
		return items[a].last > items[b].last // newer first on ties
	})

	res := make([]string, len(items))
	for i, it := range items {
		res[i] = it.cmd
	}
	return res
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

	source := sourceBookmarks
	sourceName := func() string {
		if source == sourceHistory {
			return "history"
		}
		return "bookmarks"
	}

	// A row's display text is what the user sees and fuzzy-matches against;
	// cmd is the actual command that runs when selected.
	type row struct {
		display string
		cmd     string
	}
	sourceRows := func() []row {
		if source == sourceHistory {
			rs := make([]row, len(history))
			for i, c := range history {
				rs[i] = row{c, c}
			}
			return rs
		}
		bms := bookmarks[cwd]
		rs := make([]row, len(bms))
		for i, b := range bms {
			disp := b.Command
			if b.Label != "" {
				disp = b.Label + "  ·  " + b.Command
			}
			rs[i] = row{disp, b.Command}
		}
		return rs
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

	const helpText = "  ↵ run   ⌃A add   ⌃R label   ⌃B bookmark   ⌃D delete   ⌃T toggle   esc quit"
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
		panel.SetTitle(fmt.Sprintf(" cli-bookmark · %s (%d) ", sourceName(), len(sourceRows())))
	}

	// labelMode repurposes the input field as a prompt for naming a bookmark.
	labelMode := false
	labelCmd := ""

	refreshList := func() {
		text := strings.TrimSpace(input.GetText())
		list.Clear()
		for _, r := range sourceRows() {
			if text != "" && !fuzzy.Match(text, r.display) {
				continue
			}
			// The command is stashed in the (hidden) secondary text.
			list.AddItem(r.display, r.cmd, 0, nil)
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

	// selected returns the command to act on: the highlighted row's command, or
	// the typed text when the list is empty.
	selected := func() string {
		if list.GetItemCount() == 0 {
			return strings.TrimSpace(input.GetText())
		}
		_, cmd := list.GetItemText(list.GetCurrentItem())
		return cmd
	}

	addBookmark := func(cmd string) bool {
		for _, existing := range bookmarks[cwd] {
			if existing.Command == cmd {
				flashStatus("Already bookmarked")
				return false
			}
		}
		bookmarks[cwd] = append(bookmarks[cwd], Bookmark{Command: cmd})
		if err := saveBookmarks(bookmarks); err != nil {
			flashStatus("Save error: " + err.Error())
			return false
		}
		return true
	}

	bookmarkCurrent := func() {
		cmd := selected()
		if cmd == "" {
			return
		}
		if addBookmark(cmd) {
			flashStatus("Bookmarked: " + cmd)
			if source == sourceBookmarks {
				refreshList()
			}
		}
	}

	addTyped := func() {
		cmd := strings.TrimSpace(input.GetText())
		if cmd == "" {
			flashStatus("Type a command first")
			return
		}
		if addBookmark(cmd) {
			flashStatus("Added: " + cmd)
			input.SetText("")
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
		for _, b := range items {
			if b.Command == cmd && !removed {
				removed = true
				continue
			}
			out = append(out, b)
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

	exitLabel := func() {
		labelMode = false
		labelCmd = ""
		input.SetLabel(" › ")
		input.SetText("") // ChangedFunc (now out of label mode) refreshes the list
	}

	startLabel := func() {
		if source != sourceBookmarks {
			flashStatus("Switch to bookmarks (⌃T) to label")
			return
		}
		if list.GetItemCount() == 0 {
			return
		}
		labelCmd = selected()
		existing := ""
		for _, b := range bookmarks[cwd] {
			if b.Command == labelCmd {
				existing = b.Label
				break
			}
		}
		labelMode = true
		input.SetLabel(" label › ")
		input.SetText(existing)
		flashStatus("Label for: " + labelCmd)
	}

	commitLabel := func() {
		label := strings.TrimSpace(input.GetText())
		bms := bookmarks[cwd]
		for i := range bms {
			if bms[i].Command == labelCmd {
				bms[i].Label = label
				break
			}
		}
		bookmarks[cwd] = bms
		if err := saveBookmarks(bookmarks); err != nil {
			flashStatus("Save error: " + err.Error())
		} else if label == "" {
			flashStatus("Label cleared")
		} else {
			flashStatus("Labeled: " + label)
		}
		exitLabel()
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
		if labelMode {
			return
		}
		refreshList()
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if labelMode {
			if event.Key() == tcell.KeyEsc {
				exitLabel()
				return nil
			}
			return event // let the user type the label; Enter -> DoneFunc
		}
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
		case tcell.KeyCtrlA:
			addTyped()
			return nil
		case tcell.KeyCtrlR:
			startLabel()
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
		if labelMode {
			commitLabel()
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
