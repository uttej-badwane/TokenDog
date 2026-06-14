// Command tokendog-tray is a cross-platform (Windows / Linux) system-tray
// companion to TokenDog. It shows your Claude API spend — today / month /
// lifetime — with TokenDog's savings alongside, by polling `td spend --json`.
//
// It is the Windows/Linux equivalent of the native macOS menu-bar app in
// ../macos/TokenDogBar. It also runs on macOS (handy for development), but the
// native app is preferred there.
//
// This lives in its own Go module so the system-tray dependency (which needs
// cgo) never touches the main `td` build, which stays CGO-free.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"fyne.io/systray"

	"tokendog-tray/icon"
)

// Menu items kept around so the poller can update them in place.
var (
	mToday     *systray.MenuItem
	mMonth     *systray.MenuItem
	mLifetime  *systray.MenuItem
	mNote      *systray.MenuItem
	mSavedLife *systray.MenuItem
	mShare     *systray.MenuItem
	mRefresh   *systray.MenuItem
	mReport    *systray.MenuItem
	mQuit      *systray.MenuItem
)

const refreshInterval = 60 * time.Second

func main() {
	// `--selftest` runs the data path (locate td → td spend --json → decode)
	// and prints the result without opening a tray. Handy for CI and for users
	// checking their td install works with the tray.
	selftest := flag.Bool("selftest", false, "run the data path once and print the result, then exit")
	flag.Parse()
	if *selftest {
		runSelftest()
		return
	}
	systray.Run(onReady, func() {})
}

func runSelftest() {
	fmt.Fprintf(os.Stderr, "td path: %s\n", orNone(tdPath()))
	r, err := fetchReport()
	if err != nil {
		fmt.Fprintf(os.Stderr, "selftest failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK  schema=%d td=%s\n", r.Schema, r.TDVersion)
	fmt.Printf("    spend  today=%s month=%s lifetime=%s available=%v\n",
		money(r.Spend.Today), money(r.Spend.Month), money(r.Spend.Lifetime), r.Spend.Available)
	fmt.Printf("    saved  lifetime=%s share=%.1f%%\n", microMoney(r.Saved.Lifetime), r.SharePct)
}

func orNone(s string) string {
	if s == "" {
		return "<not found>"
	}
	return s
}

func onReady() {
	systray.SetIcon(icon.Data())
	systray.SetTitle("TokenDog")
	systray.SetTooltip("TokenDog — Claude spend")

	mToday = addDisabled("Spent today: …")
	mMonth = addDisabled("This month: …")
	mLifetime = addDisabled("Lifetime: …")
	mNote = addDisabled("")
	systray.AddSeparator()
	mSavedLife = addDisabled("TD saved: …")
	mShare = addDisabled("TD share: …")
	systray.AddSeparator()
	mRefresh = systray.AddMenuItem("Refresh now", "Re-read spend now")
	mReport = systray.AddMenuItem("Open full report…", "Open `td gain --by-model` in a terminal")
	systray.AddSeparator()
	mQuit = systray.AddMenuItem("Quit TokenDog Tray", "Exit")

	go loop()
}

func addDisabled(title string) *systray.MenuItem {
	it := systray.AddMenuItem(title, "")
	it.Disable()
	return it
}

func loop() {
	refresh()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			refresh()
		case <-mRefresh.ClickedCh:
			refresh()
		case <-mReport.ClickedCh:
			openReport()
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// refresh fetches a fresh report and updates the tray title, tooltip, and menu.
// SetTitle shows text next to the icon on Linux (and is a harmless no-op on
// Windows, where the tooltip + menu carry the numbers instead).
func refresh() {
	rep, err := fetchReport()
	if err != nil {
		renderError(err)
		return
	}

	// Recover from any previously hidden rows.
	for _, it := range []*systray.MenuItem{mToday, mMonth, mLifetime, mNote, mSavedLife, mShare} {
		it.Show()
	}

	if rep.Spend.Available {
		mToday.SetTitle("Spent today: " + money(rep.Spend.Today))
		mMonth.SetTitle("This month: " + money(rep.Spend.Month))
		mLifetime.SetTitle("Lifetime: " + money(rep.Spend.Lifetime))
		mNote.SetTitle("(via Claude usage logs)")
		systray.SetTitle(moneyShort(rep.Spend.Today) + " today")
		systray.SetTooltip(fmt.Sprintf("Claude spend — today %s · month %s · lifetime %s",
			money(rep.Spend.Today), money(rep.Spend.Month), money(rep.Spend.Lifetime)))
	} else {
		mToday.SetTitle("No Claude usage logs found")
		mMonth.Hide()
		mLifetime.Hide()
		mNote.SetTitle("Showing TokenDog savings")
		systray.SetTitle(moneyShort(rep.Saved.Lifetime) + " saved")
		systray.SetTooltip("TokenDog saved " + money(rep.Saved.Lifetime) + " (no Claude logs found)")
	}

	mSavedLife.SetTitle("TD saved lifetime: " + microMoney(rep.Saved.Lifetime))
	if rep.SharePct > 0 {
		mShare.SetTitle(fmt.Sprintf("TD share of bill: %.1f%%", rep.SharePct))
	} else {
		mShare.Hide()
	}
}

func renderError(err error) {
	systray.SetTitle("td?")
	systray.SetTooltip("TokenDog: " + err.Error())
	mToday.SetTitle("TokenDog (td) not found or errored")
	if err == errTDNotFound {
		mMonth.SetTitle("Install td and ensure it's on PATH")
		mMonth.Show()
	} else {
		mMonth.SetTitle("Run `td spend` manually to debug")
		mMonth.Show()
	}
	mLifetime.Hide()
	mNote.Hide()
	mSavedLife.Hide()
	mShare.Hide()
}
