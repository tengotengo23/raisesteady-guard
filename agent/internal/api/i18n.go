package api

import (
	"net/http"
	"strings"
)

// Langs are the parent page languages, in the order of the switch.
var Langs = []struct{ Code, Name string }{{"en", "English"}, {"ka", "ქართული"}, {"ru", "Русский"}}

var strs = map[string]map[string]string{
	"title":           {"en": "Parent settings", "ka": "მშობლის პარამეტრები", "ru": "Настройки для родителей"},
	"login_lead":      {"en": "Enter your PIN to see and change the settings.", "ka": "პარამეტრების სანახავად და შესაცვლელად შეიყვანეთ PIN-კოდი.", "ru": "Введите PIN-код, чтобы увидеть и изменить настройки."},
	"pin":             {"en": "PIN", "ka": "PIN-კოდი", "ru": "PIN-код"},
	"login":           {"en": "Open settings", "ka": "პარამეტრების გახსნა", "ru": "Открыть настройки"},
	"wrong_pin":       {"en": "That PIN is not right. Tries left: %d.", "ka": "PIN-კოდი არასწორია. დარჩენილი ცდები: %d.", "ru": "Неверный PIN-код. Осталось попыток: %d."},
	"locked":          {"en": "Too many wrong tries. Please wait %d minutes and try again.", "ka": "ძალიან ბევრი არასწორი ცდა. მოიცადეთ %d წუთი და სცადეთ თავიდან.", "ru": "Слишком много неверных попыток. Подождите %d мин. и попробуйте снова."},
	"stale":           {"en": "This form is out of date. Please try again.", "ka": "ფორმა მოძველდა. გთხოვთ, სცადოთ თავიდან.", "ru": "Форма устарела. Попробуйте ещё раз."},
	"logout":          {"en": "Log out", "ka": "გასვლა", "ru": "Выйти"},
	"for_account":     {"en": "Settings for", "ka": "პარამეტრები ანგარიშისთვის:", "ru": "Настройки для"},
	"everyone":        {"en": "Everyone on this PC", "ka": "ამ კომპიუტერის ყველა მომხმარებელი", "ru": "Все пользователи этого компьютера"},
	"sec_limits":      {"en": "Age and daily time", "ka": "ასაკი და დღიური დრო", "ru": "Возраст и время в день"},
	"age":             {"en": "Age", "ka": "ასაკი", "ru": "Возраст"},
	"years":           {"en": "%s years", "ka": "%s წელი", "ru": "%s лет"},
	"browse_min":      {"en": "Browsing time a day, in minutes", "ka": "ინტერნეტის დრო დღეში, წუთებში", "ru": "Время в интернете в день, в минутах"},
	"video_min":       {"en": "Video time a day, in minutes", "ka": "ვიდეოს დრო დღეში, წუთებში", "ru": "Время на видео в день, в минутах"},
	"limit_hint":      {"en": "Time on the learning sites in the bookmarks does not count.", "ka": "სანიშნეებში არსებულ სასწავლო საიტებზე გატარებული დრო არ ითვლება.", "ru": "Время на учебных сайтах из закладок не считается."},
	"bedtime":         {"en": "Bedtime", "ka": "ძილის დრო", "ru": "Время сна"},
	"starts":          {"en": "Starts", "ka": "იწყება", "ru": "Начало"},
	"ends":            {"en": "Ends", "ka": "მთავრდება", "ru": "Конец"},
	"focus":           {"en": "Focus mode: no feeds on YouTube and scroll sites", "ka": "ფოკუსის რეჟიმი: ლენტის გარეშე YouTube-სა და სოციალურ ქსელებში", "ru": "Режим фокуса: без ленты на YouTube и в соцсетях"},
	"focus_note":      {"en": "Shows a search box and videos from chosen learning channels instead of the feeds, also on the new tab page. The videos are loaded from YouTube.", "ka": "ლენტის ნაცვლად აჩვენებს საძიებო ველს და შერჩეული საგანმანათლებლო არხების ვიდეოებს, ახალი ჩანართის გვერდზეც. ვიდეოები YouTube-იდან იტვირთება.", "ru": "Вместо ленты показывает поле поиска и видео выбранных обучающих каналов, также на странице новой вкладки. Видео загружаются с YouTube."},
	"adblock":         {"en": "Ad blocker", "ka": "რეკლამის ბლოკირება", "ru": "Блокировка рекламы"},
	"family_dns":      {"en": "Family DNS filter (AdGuard DNS Family)", "ka": "საოჯახო DNS-ფილტრი (AdGuard DNS Family)", "ru": "Семейный DNS-фильтр (AdGuard DNS Family)"},
	"family_dns_note": {"en": "Blocks adult sites, phishing and many ads. AdGuard DNS, a free public service, sees the names of the sites your child opens and your internet address, as any DNS service does. If it cannot be reached, sites do not open until it is back or you switch it off here. Clear the box to use your internet provider's DNS; the extension's own lists still work.", "ka": "ბლოკავს ზრდასრულთა საიტებს, ფიშინგს და რეკლამის დიდ ნაწილს. AdGuard DNS, უფასო საჯარო სერვისი, ხედავს იმ საიტების სახელებს, რომლებსაც ბავშვი ხსნის, და თქვენს ინტერნეტ-მისამართს, როგორც ნებისმიერი DNS-სერვისი. თუ ის მიუწვდომელია, საიტები არ გაიხსნება, სანამ ის არ აღდგება ან აქ არ გამორთავთ ფილტრს. მონიშვნა მოხსენით, რომ ინტერნეტპროვაიდერის DNS გამოიყენოთ; გაფართოების საკუთარი სიები მაინც იმუშავებს.", "ru": "Блокирует сайты для взрослых, фишинг и много рекламы. AdGuard DNS, бесплатный публичный сервис, видит названия сайтов, которые открывает ребёнок, и ваш интернет-адрес, как любой DNS-сервис. Если он недоступен, сайты не откроются, пока он не заработает или вы не выключите фильтр здесь. Снимите галочку, чтобы пользоваться DNS интернет-провайдера; собственные списки расширения продолжат работать."},
	"allow":           {"en": "Always allowed sites, one per line", "ka": "ყოველთვის დაშვებული საიტები, თითო ხაზზე ერთი", "ru": "Всегда разрешённые сайты, по одному в строке"},
	"block":           {"en": "Blocked sites, one per line", "ka": "დაბლოკილი საიტები, თითო ხაზზე ერთი", "ru": "Заблокированные сайты, по одному в строке"},
	"save":            {"en": "Save", "ka": "შენახვა", "ru": "Сохранить"},
	"defaults":        {"en": "Use the recommended times for this age", "ka": "ამ ასაკისთვის რეკომენდებული დროის დაყენება", "ru": "Поставить рекомендуемое время для этого возраста"},
	"saved":           {"en": "Saved. It applies now.", "ka": "შენახულია და უკვე მოქმედებს.", "ru": "Сохранено и уже действует."},
	"bad_input":       {"en": "Please check the values: nothing was saved.", "ka": "გთხოვთ, შეამოწმოთ მნიშვნელობები: არაფერი შენახულა.", "ru": "Проверьте значения: ничего не сохранено."},
	"sec_report":      {"en": "Last 7 days", "ka": "ბოლო 7 დღე", "ru": "Последние 7 дней"},
	"col_day":         {"en": "Day", "ka": "დღე", "ru": "День"},
	"col_browse":      {"en": "Browsing", "ka": "ინტერნეტი", "ru": "Интернет"},
	"col_video":       {"en": "Video", "ka": "ვიდეო", "ru": "Видео"},
	"col_learn":       {"en": "Learning", "ka": "სწავლა", "ru": "Учёба"},
	"col_blocked":     {"en": "Blocked pages", "ka": "დაბლოკილი გვერდები", "ru": "Заблокировано страниц"},
	"min":             {"en": "min", "ka": "წთ", "ru": "мин"},
	"top_sites":       {"en": "Most-used sites", "ka": "ყველაზე ხშირად გამოყენებული საიტები", "ru": "Самые посещаемые сайты"},
	"no_data":         {"en": "Nothing yet.", "ka": "ჯერ არაფერია.", "ru": "Пока ничего нет."},
	"sec_browsers":    {"en": "Browsers on this PC", "ka": "ბრაუზერები ამ კომპიუტერზე", "ru": "Браузеры на этом компьютере"},
	"no_browsers":     {"en": "No browser found yet. The list updates every 5 minutes.", "ka": "ბრაუზერი ჯერ ვერ მოიძებნა. სია ყოველ 5 წუთში ახლდება.", "ru": "Браузеры пока не найдены. Список обновляется каждые 5 минут."},
	"sec_accounts":    {"en": "Protected Windows accounts", "ka": "დაცული Windows-ანგარიშები", "ru": "Защищённые учётные записи Windows"},
	"admin_note":      {"en": "administrator: can remove the protection", "ka": "ადმინისტრატორი: შეუძლია დაცვის მოხსნა", "ru": "администратор: может снять защиту"},
	"everyone_note":   {"en": "Only if the child has no own account. Time limits then apply to everyone, you too.", "ka": "მხოლოდ მაშინ, თუ ბავშვს საკუთარი ანგარიში არ აქვს. ამ შემთხვევაში დროის ლიმიტი ყველაზე მოქმედებს, თქვენზეც.", "ru": "Только если у ребёнка нет своей учётной записи. Тогда ограничения действуют на всех, и на вас тоже."},
	"no_accounts":     {"en": "Choose at least one account.", "ka": "აირჩიეთ ერთი ანგარიში მაინც.", "ru": "Выберите хотя бы одну учётную запись."},
	"sec_pin":         {"en": "Change the PIN", "ka": "PIN-კოდის შეცვლა", "ru": "Сменить PIN-код"},
	"pin_now":         {"en": "Current PIN", "ka": "ამჟამინდელი PIN-კოდი", "ru": "Текущий PIN-код"},
	"pin_new":         {"en": "New PIN, 4 to 8 digits", "ka": "ახალი PIN-კოდი, 4-დან 8 ციფრამდე", "ru": "Новый PIN-код, от 4 до 8 цифр"},
	"pin_again":       {"en": "New PIN again", "ka": "გაიმეორეთ ახალი PIN-კოდი", "ru": "Повторите новый PIN-код"},
	"pin_changed":     {"en": "The PIN is changed.", "ka": "PIN-კოდი შეიცვალა.", "ru": "PIN-код изменён."},
	"pin_mismatch":    {"en": "The new PINs are not the same, or not 4 to 8 digits.", "ka": "ახალი PIN-კოდები არ ემთხვევა ან არ შედგება 4-8 ციფრისგან.", "ru": "Новые PIN-коды не совпадают или не состоят из 4–8 цифр."},
	"honest":          {"en": "No filter is perfect: stay close and talk with your child about what they see online. The child needs a standard Windows account; an administrator can remove this protection.", "ka": "არცერთი ფილტრი არ არის სრულყოფილი: იყავით ახლოს და ესაუბრეთ ბავშვს იმაზე, რასაც ინტერნეტში ხედავს. ბავშვს სჭირდება Windows-ის სტანდარტული ანგარიში; ადმინისტრატორს შეუძლია ამ დაცვის მოხსნა.", "ru": "Ни один фильтр не идеален: будьте рядом и говорите с ребёнком о том, что он видит в интернете. Ребёнку нужна стандартная учётная запись Windows; администратор может снять эту защиту."},
	"by_reason":       {"en": "Blocked pages by reason", "ka": "დაბლოკილი გვერდები მიზეზის მიხედვით", "ru": "Заблокированные страницы по причинам"},
	"reason_adult":    {"en": "Adult content", "ka": "ზრდასრულთა კონტენტი", "ru": "Контент для взрослых"},
	"reason_gambling": {"en": "Gambling", "ka": "აზარტული თამაშები", "ru": "Азартные игры"},
	"reason_bypass":   {"en": "Ways around the filter (proxies, VPNs)", "ka": "ფილტრის გვერდის ავლის საშუალებები (პროქსი, VPN)", "ru": "Обход фильтра (прокси, VPN)"},
	"reason_social":   {"en": "Social networks", "ka": "სოციალური ქსელები", "ru": "Соцсети"},
	"reason_content":  {"en": "Page content not right for the age", "ka": "ასაკისთვის შეუფერებელი შინაარსი", "ru": "Содержимое не по возрасту"},
	"reason_video":    {"en": "Videos", "ka": "ვიდეოები", "ru": "Видео"},
	"reason_time":     {"en": "Daily time used up", "ka": "დღიური დრო ამოიწურა", "ru": "Время на день закончилось"},
	"reason_bedtime":  {"en": "Bedtime", "ka": "ძილის დრო", "ru": "Время сна"},
	"reason_parent":   {"en": "Blocked by you", "ka": "თქვენ მიერ დაბლოკილი", "ru": "Заблокировано вами"},
	"st_active":       {"en": "Active · extension seen %s", "ka": "აქტიურია · გაფართოება ბოლოს ჩანდა %s", "ru": "Работает · расширение было активно %s"},
	"st_set":          {"en": "Settings set · extension not active yet", "ka": "პარამეტრები დაყენებულია · გაფართოება ჯერ არ მუშაობს", "ru": "Настройки заданы · расширение пока не работает"},
	"st_waiting":      {"en": "Settings follow at the child's next sign-in", "ka": "პარამეტრები დაყენდება ბავშვის შემდეგი შესვლისას", "ru": "Настройки применятся при следующем входе ребёнка"},
	"st_closed":       {"en": "Closed · not supported", "ka": "იხურება · არ არის მხარდაჭერილი", "ru": "Закрывается · не поддерживается"},
	"ago_now":         {"en": "just now", "ka": "ახლახან", "ru": "только что"},
	"ago_min":         {"en": "%d min ago", "ka": "%d წუთის წინ", "ru": "%d мин назад"},
	"tz":              {"en": "Time zone", "ka": "სასაათო სარტყელი", "ru": "Часовой пояс"},
	"tz_note":         {"en": "Summer time is not followed: change this when the clocks change.", "ka": "ზაფხულის დრო ავტომატურად არ იცვლება: შეცვალეთ ეს, როცა საათი გადაიწევს.", "ru": "Летнее время не учитывается: измените это, когда переводят часы."},
	"pin_today":       {"en": "%d wrong PIN tries today, last at %s.", "ka": "დღეს PIN-კოდი %d-ჯერ შეიყვანეს არასწორად, ბოლოს %s-ზე.", "ru": "Сегодня неверный PIN-код вводили %d раз, последний раз в %s."},
	"sec_privacy":     {"en": "What is kept and who sees it", "ka": "რა ინახება და ვინ ხედავს", "ru": "Что хранится и кто это видит"},
	"privacy_here":    {"en": "What stays on this PC: minutes per day, the names of sites visited (no full addresses) and blocked pages, for 30 days. Nothing is sent to RaiseSteady.", "ka": "რა რჩება ამ კომპიუტერზე: დღიური წუთები, ნანახი საიტების სახელები (სრული მისამართების გარეშე) და დაბლოკილი გვერდები, 30 დღის განმავლობაში. RaiseSteady-ს არაფერი ეგზავნება.", "ru": "Что остаётся на этом компьютере: минуты за день, названия посещённых сайтов (без полных адресов) и заблокированные страницы — 30 дней. В RaiseSteady ничего не отправляется."},
	"privacy_out":     {"en": "What leaves this PC: while the family DNS filter is on, your child's browsers look up site names through AdGuard DNS Family (a public filter); while focus mode is on, the chosen videos are loaded from YouTube.", "ka": "რა იგზავნება კომპიუტერის გარეთ: როცა საოჯახო DNS-ფილტრი ჩართულია, თქვენი შვილის ბრაუზერები საიტების სახელებს AdGuard DNS Family-ით (საჯარო ფილტრით) ამოწმებს; როცა ფოკუსის რეჟიმი ჩართულია, შერჩეული ვიდეოები YouTube-იდან იტვირთება.", "ru": "Что отправляется за пределы компьютера: пока включён семейный DNS-фильтр, браузеры ребёнка проверяют названия сайтов через AdGuard DNS Family (публичный фильтр); пока включён режим фокуса, подобранные видео загружаются с YouTube."},
	"browsers_only":   {"en": "It protects web browsers only (Chrome, Edge, Brave, Firefox). Other browsers are closed in your child's account. Games, chat and other apps are not covered.", "ka": "იცავს მხოლოდ ვებბრაუზერებს (Chrome, Edge, Brave, Firefox). სხვა ბრაუზერები თქვენი შვილის ანგარიშში იხურება. თამაშები, ჩატები და სხვა პროგრამები არ იფარება.", "ru": "Защищает только веб-браузеры (Chrome, Edge, Brave, Firefox). Другие браузеры закрываются в учётной записи ребёнка. Игры, чаты и другие программы не охватываются."},
	"language":        {"en": "Language", "ka": "ენა", "ru": "Язык"},
}

// T returns the string for key in lang, falling back to English.
func T(lang, key string) string {
	m, ok := strs[key]
	if !ok {
		return key
	}
	if s, ok := m[lang]; ok {
		return s
	}
	return m["en"]
}

func validLang(l string) bool { return l == "en" || l == "ka" || l == "ru" }

// pickLang chooses the page language: ?lang=, then the cookie, then the
// browser's Accept-Language, then English.
func pickLang(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); validLang(l) {
		return l
	}
	if c, err := r.Cookie("kg_lang"); err == nil && validLang(c.Value) {
		return c.Value
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if len(tag) >= 2 && validLang(tag[:2]) {
			return tag[:2]
		}
	}
	return "en"
}
