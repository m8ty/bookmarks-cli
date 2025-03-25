package main

import (
	"encoding/json"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rivo/tview"

	// "sort"
	"strings"
)

type Bookmarks map[string][]string // key is a string that has an array of strings

var bookmarksFile = filepath.Join(os.Getenv("HOME"), ".config", "bookmarks", "saved.json")

func main() {
	app := tview.NewApplication()

	recentCommands := []string{} // Command list to search through
	savedCommands := []string{}  // Command list to search through
	currentWDCommands := []string{}

	// Ensure bookmark directory exists
	os.MkdirAll(filepath.Dir(bookmarksFile), 0755)

	// Function to load shell history and update recent commands
	updateRecentCommands := func() {
		output, _ := exec.Command("bash", "-c", "cat ~/.zsh_history | cut -d ';' -f 2- | tail -n 20").Output()
		commands := strings.Split(string(output), "\n")
		for _, cmd := range commands {
			if cmd != "" {
				recentCommands = append(recentCommands, cmd) // Add to search list
			}
		}
	}

	// Function to load bookmarks
	updateBookmarks := func() {
		data, err := os.ReadFile(bookmarksFile)
		// fmt.Println(string(data))
		if err == nil {
			var bookmarks Bookmarks
			json.Unmarshal(data, &bookmarks)

			// Display commands from bookmarks in the recent list
			for _, commands := range bookmarks {
				for _, command := range commands {
					savedCommands = append(savedCommands, command) // Add to search list
				}
			}
		}
	}

	saveBookmarks := func(bookmarks Bookmarks) error {
		data, err := json.MarshalIndent(bookmarks, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(bookmarksFile, data, 0644)
	}

	getBookmarks := func() Bookmarks {
		var bookmarks Bookmarks
		data, err := os.ReadFile(bookmarksFile)
		if err == nil {
			json.Unmarshal(data, &bookmarks)
		}
		return bookmarks
	}

	getCWCommands := func() {
		data, err := os.ReadFile(bookmarksFile)
		if err == nil {
			var bookmarks Bookmarks
			json.Unmarshal(data, &bookmarks)
			cwd, _ := os.Getwd()
			for _, command := range bookmarks[cwd] {
				currentWDCommands = append(currentWDCommands, command) // Add to search list
			}
		}
	}

	currentWDList := tview.NewList()
	getCWCommands()
	for _, command := range currentWDCommands {
		currentWDList.AddItem(command, "", 0, nil)
	}

	inputFieldForBookmarks := tview.NewInputField()
	inputFieldForBookmarks.SetLabel("Search bookmarks: ")
	inputFieldForBookmarks.SetAutocompleteFunc(func(currentText string) (entries []string) {
		if len(currentText) == 0 {
			return
		}
		bookmarks := getBookmarks()
		cwd, _ := os.Getwd()
		fmt.Println(cwd)
		entries = fuzzy.Find(currentText, bookmarks[cwd])
		if len(entries) <= 1 {
			entries = nil
		}
		return
	})
	inputFieldForBookmarks.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			command := inputFieldForBookmarks.GetText()
			exec.Command("bash", "-c", command).Run()
			copyCmd := exec.Command("pbcopy")
			copyCmd.Stdin = strings.NewReader(command)
			err := copyCmd.Run()
			app.Stop()
			if err != nil {
				// Handle error (optional)
				fmt.Println("Error copying to clipboard:", err)
			}
		}

	})

	// Create input field
	inputField := tview.NewInputField().SetFieldWidth(20)
	inputField.SetLabel("Search history: ")
	inputField.SetFieldBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	inputField.SetAutocompleteFunc(func(currentText string) (entries []string) {
		if len(currentText) == 0 {
			return
		}
		entries = fuzzy.Find(currentText, recentCommands)
		if len(entries) <= 1 {
			entries = nil
		}
		return
	})
	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			command := inputField.GetText()
			exec.Command("bash", "-c", command).Run()
			copyCmd := exec.Command("pbcopy")
			copyCmd.Stdin = strings.NewReader(command)
			err := copyCmd.Run()
			app.Stop()
			if err != nil {
				// Handle error (optional)
				fmt.Println("Error copying to clipboard:", err)
			}
		}

	})
	inputField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'b' {
			command := inputField.GetText()
			if command == "" {
				return event
			}

			// Get current working directory
			cwd, err := os.Getwd()
			if err != nil {
				inputField.SetLabel("Error getting current directory! ")
				return event
			}

			// Load existing bookmarks
			var bookmarks Bookmarks
			data, err := os.ReadFile(bookmarksFile)
			if err == nil {
				json.Unmarshal(data, &bookmarks)
			} else {
				bookmarks = make(Bookmarks)
			}

			// Add command to the current directory's bookmarks
			if _, exists := bookmarks[cwd]; !exists {
				bookmarks[cwd] = []string{}
			}
			bookmarks[cwd] = append(bookmarks[cwd], command)

			// Save updated bookmarks
			if err := saveBookmarks(bookmarks); err != nil {
				inputField.SetLabel("Error saving! ")
				return event
			}

			// Show confirmation message
			inputField.SetLabel("Saved!")
			inputField.SetText("")
			app.ForceDraw()
			return nil
		}

		if event.Key() == tcell.KeyCtrlJ {
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		}
		if event.Key() == tcell.KeyCtrlK {
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}

		return event
	})

	// recentList.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
	//
	// })

	// Initial data load
	updateRecentCommands()
	updateBookmarks()

	fmt.Println(recentCommands)
	fmt.Println(savedCommands)

	box := tview.NewFlex()
	recentList := tview.NewList()
	for _, command := range recentCommands {
		recentList.AddItem(command, "", 0, nil)
	}
	box.AddItem(recentList, 0, 1, true)

	// Layout
	flex := tview.NewFlex().
		AddItem(inputField, 0, 1, true).
		AddItem(box, 0, 3, false).AddItem(inputFieldForBookmarks, 0, 1, true).AddItem(currentWDList, 0, 1, true)

	// Run the app
	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
