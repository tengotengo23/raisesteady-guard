package main

// The setup command: the Linux counterpart of the Windows installer's pages
// (terms, age, PIN, family DNS filter), in the terminal, in English, Georgian
// or Russian (from the system language). With --yes and the flags it asks
// nothing (for scripts and tests).

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
)

var setupText = map[string]map[string]string{
	"title": {
		"en": "RaiseSteady Guard setup",
		"ka": "RaiseSteady Guard-ის დაყენება",
		"ru": "Установка RaiseSteady Guard",
	},
	"already": {
		"en": "RaiseSteady Guard is already set up on this computer. Change the settings on the parent page: http://127.0.0.1:%d/parent\nTo remove it, use your package manager, for example: sudo apt remove raisesteady-guard (Ubuntu, Debian) or sudo dnf remove raisesteady-guard (Fedora); from the .tar.gz: sudo /opt/raisesteady-guard/uninstall.sh.",
		"ka": "RaiseSteady Guard ამ კომპიუტერზე უკვე დაყენებულია. პარამეტრები შეცვალეთ მშობლის გვერდზე: http://127.0.0.1:%d/parent\nწასაშლელად გამოიყენეთ პაკეტების მენეჯერი, მაგალითად: sudo apt remove raisesteady-guard (Ubuntu, Debian) ან sudo dnf remove raisesteady-guard (Fedora); .tar.gz-დან დაყენებისას: sudo /opt/raisesteady-guard/uninstall.sh.",
		"ru": "RaiseSteady Guard уже установлен на этом компьютере. Настройки меняются на странице для родителей: http://127.0.0.1:%d/parent\nЧтобы удалить его, используйте менеджер пакетов, например: sudo apt remove raisesteady-guard (Ubuntu, Debian) или sudo dnf remove raisesteady-guard (Fedora); при установке из .tar.gz: sudo /opt/raisesteady-guard/uninstall.sh.",
	},
	"agree": {
		"en": "Type yes to agree, or press Enter to stop: ",
		"ka": "თანხმობისთვის აკრიფეთ yes, შესაჩერებლად დააჭირეთ Enter-ს: ",
		"ru": "Наберите yes, чтобы согласиться, или нажмите Enter, чтобы прервать установку: ",
	},
	"stopped": {
		"en": "Setup stopped. Nothing was changed.",
		"ka": "დაყენება შეჩერდა. არაფერი შეცვლილა.",
		"ru": "Установка остановлена. Ничего не изменено.",
	},
	"whole": {
		"en": "On Linux, browsers take these settings for every account on this computer, including yours: safe search, the family filter, no private windows, and no other browser add-ons. It works best on a computer your child uses; on a shared computer the settings apply to you too.",
		"ka": "Linux-ზე ბრაუზერები ამ პარამეტრებს ამ კომპიუტერის ყველა ანგარიშზე იყენებენ, თქვენის ჩათვლით: უსაფრთხო ძიება, საოჯახო ფილტრი, პირადი ფანჯრების გარეშე და სხვა დამატებების გარეშე. საუკეთესოა იმ კომპიუტერზე, რომელსაც ბავშვი იყენებს; საერთო კომპიუტერზე პარამეტრები თქვენზეც გავრცელდება.",
		"ru": "В Linux браузеры применяют эти настройки ко всем учётным записям этого компьютера, включая вашу: безопасный поиск, семейный фильтр, без приватных окон и без других дополнений. Лучше всего это подходит для компьютера, которым пользуется ребёнок; на общем компьютере настройки будут действовать и для вас.",
	},
	"admins": {
		"en": "These accounts are administrators and can turn the protection off: %s. Your child must not use them or know their passwords.",
		"ka": "ეს ანგარიშები ადმინისტრატორებია და დაცვის გამორთვა შეუძლიათ: %s. ბავშვმა არ უნდა გამოიყენოს ისინი და არ უნდა იცოდეს მათი პაროლები.",
		"ru": "Эти учётные записи — администраторы, они могут отключить защиту: %s. Ребёнок не должен ими пользоваться и знать их пароли.",
	},
	"found": {
		"en": "Browsers that get the safe settings: %s.",
		"ka": "ბრაუზერები, რომლებიც უსაფრთხო პარამეტრებს მიიღებენ: %s.",
		"ru": "Браузеры, которые получат безопасные настройки: %s.",
	},
	"closed": {
		"en": "These browsers will be closed whenever someone opens them on this computer: %s.",
		"ka": "ეს ბრაუზერები დაიხურება, როცა ვინმე მათ ამ კომპიუტერზე გახსნის: %s.",
		"ru": "Эти браузеры будут закрываться, когда их кто-то открывает на этом компьютере: %s.",
	},
	"age": {
		"en": "Your child's age:",
		"ka": "თქვენი შვილის ასაკი:",
		"ru": "Возраст ребёнка:",
	},
	"agePick": {
		"en": "Type a number (1 to %d): ",
		"ka": "აკრიფეთ რიცხვი (1-დან %d-მდე): ",
		"ru": "Наберите номер (от 1 до %d): ",
	},
	"pin": {
		"en": "Choose a parent PIN, 4 to 8 digits (it is not shown as you type): ",
		"ka": "აირჩიეთ მშობლის PIN-კოდი, 4-დან 8 ციფრამდე (აკრეფისას არ ჩანს): ",
		"ru": "Выберите PIN-код родителя, от 4 до 8 цифр (при наборе он не виден): ",
	},
	"pin2": {
		"en": "PIN again: ",
		"ka": "გაიმეორეთ PIN-კოდი: ",
		"ru": "Повторите PIN-код: ",
	},
	"pinBad": {
		"en": "The PIN must be 4 to 8 digits, and both must be the same.",
		"ka": "PIN-კოდი უნდა შედგებოდეს 4-8 ციფრისგან და ორივე ერთნაირი უნდა იყოს.",
		"ru": "PIN-код должен состоять из 4–8 цифр, и оба должны совпадать.",
	},
	"pinHint": {
		"en": "You need the PIN to change the settings on the parent page. Keep it to yourself.",
		"ka": "PIN-კოდი დაგჭირდებათ მშობლის გვერდზე პარამეტრების შესაცვლელად. არავის უთხრათ.",
		"ru": "PIN-код нужен, чтобы менять настройки на странице для родителей. Никому его не говорите.",
	},
	"dnsTitle": {
		"en": "Family DNS filter",
		"ka": "საოჯახო DNS-ფილტრი",
		"ru": "Семейный DNS-фильтр",
	},
	"dnsText": {
		"en": "It blocks adult sites, phishing and many ads in the browsers, and turns on safe search.\n\nAdGuard DNS, a free public service, then sees the names of the sites opened (not the pages or what is typed) and your internet address, as any DNS service does. If it cannot be reached, sites do not open until it is back or you switch it off.\n\nIf you answer n, your internet provider's usual DNS is used. Once the browser extension is installed, its own block lists work too.\n\nOnce the extension is installed, the new tab page also shows videos from chosen YouTube channels, loaded from YouTube (Focus mode). You can change both on the parent page. Nothing is sent to us.",
		"ka": "ის ბლოკავს ზრდასრულთა საიტებს, ფიშინგს და რეკლამის დიდ ნაწილს ბრაუზერებში და ჩართავს უსაფრთხო ძიებას.\n\nAdGuard DNS, უფასო საჯარო სერვისი, ხედავს გახსნილი საიტების სახელებს (არა გვერდებს ან აკრეფილ ტექსტს) და თქვენს ინტერნეტ-მისამართს, როგორც ნებისმიერი DNS-სერვისი. თუ ის მიუწვდომელია, საიტები არ გაიხსნება, სანამ ის არ აღდგება ან თქვენ არ გამორთავთ ფილტრს.\n\nთუ n-ს უპასუხებთ, გამოყენებული იქნება ინტერნეტპროვაიდერის ჩვეულებრივი DNS. გაფართოების დაყენების შემდეგ მისი საკუთარი ბლოკირების სიებიც იმუშავებს.\n\nგაფართოების დაყენების შემდეგ ახალი ჩანართის გვერდზე ასევე ჩანს შერჩეული YouTube-არხების ვიდეოები, რომლებიც YouTube-იდან იტვირთება (ფოკუსის რეჟიმი). ორივეს შეცვლა მშობლის გვერდზე შეგიძლიათ. ჩვენთან არაფერი იგზავნება.",
		"ru": "Он блокирует сайты для взрослых, фишинг и много рекламы в браузерах и включает безопасный поиск.\n\nAdGuard DNS, бесплатный публичный сервис, видит названия открытых сайтов (но не страницы и не то, что набрано) и ваш интернет-адрес, как любой DNS-сервис. Если он недоступен, сайты не откроются, пока он не заработает или вы не выключите фильтр.\n\nЕсли ответите n, будет использоваться обычный DNS интернет-провайдера. Когда будет установлено расширение браузера, заработают и его собственные списки блокировки.\n\nКогда расширение установлено, на странице новой вкладки также показываются видео выбранных каналов YouTube, которые загружаются с YouTube (режим фокуса). Оба параметра можно изменить на странице для родителей. К нам ничего не отправляется.",
	},
	"dnsAsk": {
		"en": "Use the AdGuard DNS Family filter? Recommended. [Y/n]: ",
		"ka": "გამოვიყენოთ AdGuard DNS Family-ის ფილტრი? რეკომენდებულია. [Y/n]: ",
		"ru": "Использовать фильтр AdGuard DNS Family? Рекомендуется. [Y/n]: ",
	},
	"applying": {
		"en": "Setting up the browsers...",
		"ka": "ბრაუზერების გამართვა...",
		"ru": "Настройка браузеров...",
	},
	"done": {
		"en": "Done. Close and reopen any open browsers so they take the new settings.\nParent page (settings and PIN): http://127.0.0.1:%d/parent, or RaiseSteady Guard in your app menu.",
		"ka": "მზადაა. დახურეთ და თავიდან გახსენით ღია ბრაუზერები, რომ ახალი პარამეტრები ამოქმედდეს.\nმშობლის გვერდი (პარამეტრები და PIN-კოდი): http://127.0.0.1:%d/parent, ან RaiseSteady Guard აპების მენიუში.",
		"ru": "Готово. Закройте и снова откройте браузеры, чтобы они приняли новые настройки.\nСтраница для родителей (настройки и PIN-код): http://127.0.0.1:%d/parent или RaiseSteady Guard в меню приложений.",
	},
	"failed": {
		"en": "Setup could not finish: %v",
		"ka": "დაყენება ვერ დასრულდა: %v",
		"ru": "Установка не завершилась: %v",
	},
	"needFlags": {
		"en": "No terminal to ask in: give --yes --band ID --pin-env NAME [--family-dns on|off].",
		"ka": "No terminal to ask in: give --yes --band ID --pin-env NAME [--family-dns on|off].",
		"ru": "No terminal to ask in: give --yes --band ID --pin-env NAME [--family-dns on|off].",
	},
}

var bandLabel = map[string]map[string]string{
	"3-5":   {"en": "3 to 5 years", "ka": "3-დან 5 წლამდე", "ru": "От 3 до 5 лет"},
	"6-8":   {"en": "6 to 8 years", "ka": "6-დან 8 წლამდე", "ru": "От 6 до 8 лет"},
	"9-12":  {"en": "9 to 12 years", "ka": "9-დან 12 წლამდე", "ru": "От 9 до 12 лет"},
	"13-15": {"en": "13 to 15 years", "ka": "13-დან 15 წლამდე", "ru": "От 13 до 15 лет"},
	"16-17": {"en": "16 to 17 years", "ka": "16-დან 17 წლამდე", "ru": "От 16 до 17 лет"},
}

func st(lang, key string) string {
	if t, ok := setupText[key][lang]; ok {
		return t
	}
	return setupText[key]["en"]
}

// setupIO is what the wizard reads and writes (a terminal, or test buffers).
type setupIO struct {
	in      *bufio.Reader
	out     io.Writer
	secret  func() (string, error) // reads a line without echo
	isTerm  bool
	lang    string
	getenv  func(string) string
	exePath string
}

func (s *setupIO) ask(prompt string) (string, error) {
	fmt.Fprint(s.out, prompt)
	line, err := s.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (s *setupIO) askSecret(prompt string) (string, error) {
	fmt.Fprint(s.out, prompt)
	v, err := s.secret()
	fmt.Fprintln(s.out)
	return strings.TrimSpace(v), err
}

type setupFlags struct {
	yes       bool
	band      string
	pin       string
	familyDNS string
	port      int
	xpi       string
}

// runSetup asks the questions and installs (all accounts mode).
func runSetup(sio *setupIO, f setupFlags, sys agent.System, svcs agent.Services, data *core.Data, paths agent.Paths) int {
	l := sio.lang
	out := sio.out
	if core.LoadJSON(paths.Config, &core.Config{}) == nil {
		var cfg core.Config
		_ = core.LoadJSON(paths.Config, &cfg)
		port := cfg.Port
		if port == 0 {
			port = core.DefaultPort
		}
		fmt.Fprintf(out, st(l, "already")+"\n", port)
		return exitOK
	}
	interactive := !f.yes
	if interactive && !sio.isTerm {
		fmt.Fprintln(out, st(l, "needFlags"))
		return exitUsage
	}
	fmt.Fprintf(out, "\n%s\n%s\n\n", st(l, "title"), strings.Repeat("=", len([]rune(st(l, "title")))))
	if interactive {
		fmt.Fprintln(out, strings.TrimSpace(core.Terms(l)))
		fmt.Fprintln(out)
		a, err := sio.ask(st(l, "agree"))
		if err != nil || !isYes(a) {
			fmt.Fprintln(out, st(l, "stopped"))
			return exitError
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, st(l, "whole"))
	// Administrators other than the person running setup.
	accts, _ := sys.LocalAccounts()
	var admins []string
	for _, a := range accts {
		if a.Admin && a.Name != sio.getenv("SUDO_USER") {
			admins = append(admins, a.Name)
		}
	}
	if len(admins) > 0 {
		fmt.Fprintf(out, "\n"+st(l, "admins")+"\n", strings.Join(admins, ", "))
	}
	var ok, closed []string
	for _, fd := range detectFound(sys, data) {
		if fd.Supported {
			ok = appendOnce(ok, fd.Name)
		} else {
			closed = appendOnce(closed, fd.Name)
		}
	}
	if len(ok) > 0 {
		fmt.Fprintf(out, "\n"+st(l, "found")+"\n", strings.Join(ok, ", "))
	}
	if len(closed) > 0 {
		fmt.Fprintf(out, "\n"+st(l, "closed")+"\n", strings.Join(closed, ", "))
	}
	fmt.Fprintln(out)
	band := f.band
	if band == "" && interactive {
		fmt.Fprintln(out, st(l, "age"))
		for i, b := range data.Bands.List {
			fmt.Fprintf(out, "  %d) %s\n", i+1, bandName(b.ID, l))
		}
		for band == "" {
			a, err := sio.ask(fmt.Sprintf(st(l, "agePick"), len(data.Bands.List)))
			if err != nil {
				fmt.Fprintln(out, st(l, "stopped"))
				return exitError
			}
			if n, err := strconv.Atoi(a); err == nil && n >= 1 && n <= len(data.Bands.List) {
				band = data.Bands.List[n-1].ID
			}
		}
		fmt.Fprintln(out)
	}
	pin := f.pin
	if pin == "" && interactive {
		fmt.Fprintln(out, st(l, "pinHint"))
		for pin == "" {
			p1, err := sio.askSecret(st(l, "pin"))
			if err != nil {
				fmt.Fprintln(out, st(l, "stopped"))
				return exitError
			}
			p2, err := sio.askSecret(st(l, "pin2"))
			if err != nil {
				fmt.Fprintln(out, st(l, "stopped"))
				return exitError
			}
			if p1 != p2 || !core.ValidPin(p1) {
				fmt.Fprintln(out, st(l, "pinBad"))
				continue
			}
			pin = p1
		}
		fmt.Fprintln(out)
	}
	dns := f.familyDNS
	if dns == "" && interactive {
		fmt.Fprintf(out, "%s\n\n%s\n\n", st(l, "dnsTitle"), st(l, "dnsText"))
		for dns == "" {
			a, err := sio.ask(st(l, "dnsAsk"))
			if err != nil {
				fmt.Fprintln(out, st(l, "stopped"))
				return exitError
			}
			switch strings.ToLower(a) {
			case "", "y", "yes", "д", "да", "კი", "დიახ":
				dns = "on"
			case "n", "no", "н", "нет", "არა":
				dns = "off"
			}
		}
		fmt.Fprintln(out)
	}
	if dns == "" {
		dns = "on"
	}
	if dns != "on" && dns != "off" {
		fmt.Fprintln(out, "--family-dns must be on or off")
		return exitUsage
	}
	port := f.port
	if port == 0 {
		port = core.DefaultPort
	}
	fmt.Fprintln(out, st(l, "applying"))
	_, err := agent.Install(sys, svcs, data, paths, agent.InstallOptions{
		Accounts: "all", Band: band, Pin: pin, Port: port, ExePath: sio.exePath, FirefoxXPI: f.xpi,
		FamilyDNSOff: dns == "off",
	}, nil)
	if err != nil {
		fmt.Fprintf(out, st(l, "failed")+"\n", err)
		return exitError
	}
	fmt.Fprintf(out, "\n"+st(l, "done")+"\n", port)
	return exitOK
}

func isYes(a string) bool {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "yes", "y", "да", "კი", "დიახ":
		return true
	}
	return false
}

func bandName(id, lang string) string {
	if m, ok := bandLabel[id]; ok {
		if t, ok := m[lang]; ok {
			return t
		}
		return m["en"]
	}
	return id
}

func appendOnce(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// detectFound lists the installed browsers through the Linux detector when
// the system has one.
func detectFound(sys agent.System, data *core.Data) []detect.Found {
	if d, ok := sys.(interface {
		DetectBrowsers(core.UnsupportedList) []detect.Found
	}); ok {
		return d.DetectBrowsers(data.Unsupported)
	}
	if env := sys.DetectEnv(); env != nil {
		return detect.Detect(env, data.Unsupported)
	}
	return nil
}

func newSetupIO(stdout io.Writer) *setupIO {
	exe, _ := os.Executable()
	in := bufio.NewReader(os.Stdin)
	return &setupIO{
		in: in, out: stdout, secret: func() (string, error) { return readSecret(in) }, isTerm: stdinIsTerminal(),
		getenv: os.Getenv, exePath: exe,
	}
}
