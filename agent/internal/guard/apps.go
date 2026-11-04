package guard

import (
	"path"
	"strings"

	"raisesteady-guard/agent/internal/core"
)

// PC games and chat apps (P2-160, F5). The same 3-second loop that closes
// unsupported browsers counts game time and closes apps the account may not
// use now.

// Why an app is closed.
const (
	StopBlocked  = "blocked"  // the app is set to Blocked
	StopUsed     = "used"     // the game time is used up
	StopBedtime  = "bedtime"  // it is bedtime
	StopHomework = "homework" // it is homework time
	StopPaused   = "paused"   // the parent paused the internet
)

// WarnBeforeSec is when the game-time warning shows.
const WarnBeforeSec = 5 * 60

// AppState is one account's app rules at this moment.
type AppState struct {
	Key   string            // the account (its SID, or "*")
	Modes map[string]string // app id -> core.AppAllowed, AppGame or AppBlocked
	// Stop says why game-time apps must close now (StopUsed, StopBedtime,
	// StopHomework, StopPaused), "" when they may run.
	Stop    string
	LeftSec int // game seconds left today
}

// AppRules is what the guard needs from the agent for the apps.
type AppRules interface {
	// State returns the rules of the account a process owner belongs to.
	State(sid string) (AppState, bool)
	// AddGame adds game seconds to an account's day.
	AddGame(key string, sec int)
	// Warn records the game-time warning; false when it was already shown today.
	Warn(key string) bool
	// Closed counts a closed app for the report.
	Closed(key, appID string)
}

// ClassifyApp finds the listed app a program belongs to, or nil. Windows
// matches a part of the path, the product name or the exe name; Linux the
// Flatpak id, the snap name or the program name.
func ClassifyApp(apps []core.PCApp, p, product string, linux bool) *core.PCApp {
	if p == "" {
		return nil
	}
	if linux {
		return classifyAppLinux(apps, p)
	}
	np := normPath(p)
	exe := baseName(p)
	prod := strings.TrimSpace(product)
	for i := range apps {
		a := &apps[i]
		for _, pc := range a.PathContains {
			if pc != "" && strings.Contains(np, strings.ToLower(pc)) {
				return a
			}
		}
	}
	for i := range apps {
		a := &apps[i]
		for _, x := range a.Product {
			if prod != "" && strings.EqualFold(prod, x) {
				return a
			}
		}
		for _, e := range a.Exe {
			if strings.EqualFold(exe, e) {
				return a
			}
		}
	}
	return nil
}

func classifyAppLinux(apps []core.PCApp, p string) *core.PCApp {
	p = strings.TrimPrefix(p, "unverified:")
	var flatpakID, snapName string
	switch {
	case strings.HasPrefix(p, "flatpak-claim:"):
		id, exe, _ := strings.Cut(strings.TrimPrefix(p, "flatpak-claim:"), ":")
		flatpakID = id
		if a := matchFlatpak(apps, id); a != nil {
			return a
		}
		if exe == "" || strings.HasPrefix(exe, "flatpak") {
			return nil
		}
		return classifyAppLinux(apps, exe)
	case strings.HasPrefix(p, "flatpak:"):
		flatpakID = strings.TrimPrefix(p, "flatpak:")
	case strings.HasPrefix(p, "flatpak-user:"):
		flatpakID = strings.TrimPrefix(p, "flatpak-user:")
	case strings.HasPrefix(p, "/snap/"):
		snapName, _, _ = strings.Cut(strings.TrimPrefix(p, "/snap/"), "/")
	}
	if flatpakID != "" {
		return matchFlatpak(apps, flatpakID)
	}
	for i := range apps {
		for _, s := range apps[i].Snap {
			if snapName != "" && strings.EqualFold(s, snapName) {
				return &apps[i]
			}
		}
	}
	exe := path.Base(p)
	for i := range apps {
		for _, e := range apps[i].LinuxExe {
			if exe == e {
				return &apps[i]
			}
		}
	}
	return nil
}

func matchFlatpak(apps []core.PCApp, id string) *core.PCApp {
	for i := range apps {
		for _, f := range apps[i].Flatpak {
			if strings.EqualFold(f, id) {
				return &apps[i]
			}
		}
	}
	return nil
}

var appText = map[string]map[string]string{
	StopBlocked: {
		"en": "Your parent has switched %s off on this computer, so it was closed.",
		"ka": "მშობელმა %s ამ კომპიუტერზე გამორთო, ამიტომ დაიხურა.",
		"ru": "Программа %s отключена родителем на этом компьютере, поэтому она закрыта.",
	},
	StopUsed: {
		"en": "Game time is used up for today, so %s was closed. It comes back tomorrow.",
		"ka": "დღევანდელი თამაშის დრო ამოიწურა, ამიტომ %s დაიხურა. ხვალ ისევ შეძლებ.",
		"ru": "Время для игр на сегодня закончилось, поэтому программа %s закрыта. Завтра оно будет снова.",
	},
	StopBedtime: {
		"en": "It's bedtime, so %s was closed. Sleep well.",
		"ka": "ძილის დროა, ამიტომ %s დაიხურა. ტკბილად გეძინოს.",
		"ru": "Пора спать, поэтому программа %s закрыта. Спокойной ночи.",
	},
	StopHomework: {
		"en": "It's homework time, so %s was closed. It opens again when homework time ends.",
		"ka": "საშინაო დავალების დროა, ამიტომ %s დაიხურა. დავალების დროის შემდეგ ისევ გაიხსნება.",
		"ru": "Сейчас время для домашних заданий, поэтому программа %s закрыта. Потом она снова откроется.",
	},
	StopPaused: {
		"en": "Your parent has paused games and the internet for now, so %s was closed.",
		"ka": "მშობელმა თამაშები და ინტერნეტი დროებით შეაჩერა, ამიტომ %s დაიხურა.",
		"ru": "Родитель временно приостановил игры и интернет, поэтому программа %s закрыта.",
	},
}

var warnText = map[string]string{
	"en": "Less than 5 minutes of game time left today. A good moment to save your game.",
	"ka": "დღეს თამაშის დროიდან 5 წუთზე ნაკლები დარჩა. კარგი დროა, თამაში შეინახო.",
	"ru": "На игры сегодня осталось меньше 5 минут. Самое время сохранить игру.",
}

// AppMessage is the short message shown when an app is closed.
func AppMessage(lang, why, app string) string {
	m, ok := appText[why]
	if !ok {
		m = appText[StopBlocked]
	}
	t, ok := m[lang]
	if !ok {
		t = m["en"]
	}
	return strings.Replace(t, "%s", app, 1)
}

// GameWarnMessage is the warning 5 minutes before the game time ends.
func GameWarnMessage(lang string) string {
	if t, ok := warnText[lang]; ok {
		return t
	}
	return warnText["en"]
}
