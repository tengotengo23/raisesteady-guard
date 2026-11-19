package api

import (
	"net/http"
	"strings"
)

// Langs are the parent page languages, in the order of the switch.
var Langs = []struct{ Code, Name string }{{"en", "English"}, {"ka", "ქართული"}, {"ru", "Русский"}}

var strs = map[string]map[string]string{
	// The first-run setup on the parent page (P2-194)
	"setup_title":         {"en": "Set up RaiseSteady Guard", "ka": "RaiseSteady Guard-ის დაყენება", "ru": "Настройка RaiseSteady Guard"},
	"setup_lead":          {"en": "RaiseSteady Guard is installed. Browsers take the safe settings (safe search, the family filter, no private windows) when they next start. A few short steps finish the setup. Time limits start when you choose your child's age.", "ka": "RaiseSteady Guard დაყენებულია. ბრაუზერები უსაფრთხო პარამეტრებს (უსაფრთხო ძიება, საოჯახო ფილტრი, პირადი ფანჯრების გარეშე) მომდევნო გახსნისას მიიღებენ. დაყენების დასასრულებლად რამდენიმე მოკლე ნაბიჯი დარჩა. დროის ლიმიტები მაშინ დაიწყება, როცა შვილის ასაკს აირჩევთ.", "ru": "RaiseSteady Guard установлен. Браузеры применят безопасные настройки (безопасный поиск, семейный фильтр, без приватных окон) при следующем запуске. Осталось несколько коротких шагов. Ограничения по времени начнут действовать, когда вы выберете возраст ребёнка."},
	"setup_terms_h":       {"en": "1. Terms", "ka": "1. პირობები", "ru": "1. Условия"},
	"setup_agree":         {"en": "I have read the terms and I agree.", "ka": "პირობებს გავეცანი და ვეთანხმები.", "ru": "Я прочитал(а) условия и принимаю их."},
	"setup_age_h":         {"en": "2. Your child's age", "ka": "2. თქვენი შვილის ასაკი", "ru": "2. Возраст ребёнка"},
	"setup_age_pick":      {"en": "Choose the age", "ka": "აირჩიეთ ასაკი", "ru": "Выберите возраст"},
	"setup_age_hint":      {"en": "The age sets the daily time, the bedtime and what is allowed. You can change all of it later on this page.", "ka": "ასაკი განსაზღვრავს დღიურ დროს, ძილის დროს და იმას, რაც ნებადართულია. ყველაფრის შეცვლა მოგვიანებით ამ გვერდზე შეგიძლიათ.", "ru": "От возраста зависят время в день, время сна и то, что разрешено. Всё это можно изменить позже на этой странице."},
	"setup_pin_h":         {"en": "3. Parent PIN", "ka": "3. მშობლის PIN-კოდი", "ru": "3. PIN-код родителя"},
	"setup_pin1":          {"en": "Choose a PIN, 4 to 8 digits", "ka": "აირჩიეთ PIN-კოდი, 4-დან 8 ციფრამდე", "ru": "Выберите PIN-код, от 4 до 8 цифр"},
	"setup_pin2":          {"en": "The PIN again", "ka": "გაიმეორეთ PIN-კოდი", "ru": "Повторите PIN-код"},
	"setup_pin_hint":      {"en": "You need the PIN to change the settings on this page. Keep it to yourself. If the browser offers to save it, choose Never.", "ka": "PIN-კოდი დაგჭირდებათ ამ გვერდზე პარამეტრების შესაცვლელად. არავის უთხრათ. თუ ბრაუზერი მის შენახვას შემოგთავაზებთ, აირჩიეთ „არასოდეს“.", "ru": "PIN-код нужен, чтобы менять настройки на этой странице. Никому его не говорите. Если браузер предложит его сохранить, выберите «Никогда»."},
	"setup_dns_h":         {"en": "Family filter", "ka": "საოჯახო ფილტრი", "ru": "Семейный фильтр"},
	"setup_submit":        {"en": "Finish the setup", "ka": "დაყენების დასრულება", "ru": "Завершить настройку"},
	"setup_know_h":        {"en": "Good to know", "ka": "სასარგებლო ინფორმაცია", "ru": "Полезно знать"},
	"setup_whole":         {"en": "On Linux, browsers take these settings for every account on this computer, including yours: safe search, the family filter, no private windows, and no other browser add-ons. It is made for the computer your child uses.", "ka": "Linux-ზე ბრაუზერები ამ პარამეტრებს ამ კომპიუტერის ყველა ანგარიშზე იყენებენ, თქვენის ჩათვლით: უსაფრთხო ძიება, საოჯახო ფილტრი, პირადი ფანჯრების გარეშე და სხვა დამატებების გარეშე. განკუთვნილია იმ კომპიუტერისთვის, რომელსაც ბავშვი იყენებს.", "ru": "В Linux браузеры применяют эти настройки ко всем учётным записям этого компьютера, включая вашу: безопасный поиск, семейный фильтр, без приватных окон и без других дополнений. Программа рассчитана на компьютер, которым пользуется ребёнок."},
	"setup_admins":        {"en": "These accounts are administrators and can turn the protection off: %s. Your child must not use them or know their passwords.", "ka": "ეს ანგარიშები ადმინისტრატორებია და დაცვის გამორთვა შეუძლიათ: %s. ბავშვმა არ უნდა გამოიყენოს ისინი და არ უნდა იცოდეს მათი პაროლები.", "ru": "Эти учётные записи — администраторы, они могут отключить защиту: %s. Ребёнок не должен ни пользоваться ими, ни знать их пароли."},
	"managed_note":        {"en": "Chrome and browsers like it show \"Your browser is managed by your organization\". That is RaiseSteady Guard: it is how the safe settings stay locked.", "ka": "Chrome და მსგავსი ბრაუზერები აჩვენებენ წარწერას „თქვენს ბრაუზერს მართავს თქვენი ორგანიზაცია“. ეს RaiseSteady Guard-ია: ასე რჩება უსაფრთხო პარამეტრები დაცული ცვლილებისგან.", "ru": "Chrome и похожие браузеры показывают надпись «Этим браузером управляет ваша организация». Это RaiseSteady Guard: так безопасные настройки защищены от изменения."},
	"setup_done":          {"en": "The setup is finished and the limits apply now. Close each open browser and open it again, so that it takes the safe settings and the RaiseSteady Guard extension.", "ka": "დაყენება დასრულდა და ლიმიტები უკვე მოქმედებს. დახურეთ ყველა ღია ბრაუზერი და თავიდან გახსენით, რომ მან უსაფრთხო პარამეტრები და RaiseSteady Guard-ის გაფართოება მიიღოს.", "ru": "Настройка завершена, ограничения уже действуют. Закройте каждый открытый браузер и откройте его снова: так он получит безопасные настройки и расширение RaiseSteady Guard."},
	"setup_agree_err":     {"en": "Please tick the box to agree to the terms.", "ka": "პირობებზე თანხმობისთვის მონიშნეთ ველი.", "ru": "Отметьте галочку, чтобы принять условия."},
	"setup_age":           {"en": "Please choose your child's age.", "ka": "გთხოვთ, აირჩიოთ შვილის ასაკი.", "ru": "Выберите возраст ребёнка."},
	"setup_pin":           {"en": "The PIN must be 4 to 8 digits, and both must be the same.", "ka": "PIN-კოდი უნდა შედგებოდეს 4-8 ციფრისგან და ორივე ერთნაირი უნდა იყოს.", "ru": "PIN-код должен состоять из 4–8 цифр, и оба должны совпадать."},
	"title":               {"en": "Parent settings", "ka": "მშობლის პარამეტრები", "ru": "Настройки для родителей"},
	"login_lead":          {"en": "Enter your PIN to see and change the settings.", "ka": "პარამეტრების სანახავად და შესაცვლელად შეიყვანეთ PIN-კოდი.", "ru": "Введите PIN-код, чтобы увидеть и изменить настройки."},
	"pin":                 {"en": "PIN", "ka": "PIN-კოდი", "ru": "PIN-код"},
	"login":               {"en": "Open settings", "ka": "პარამეტრების გახსნა", "ru": "Открыть настройки"},
	"wrong_pin":           {"en": "That PIN is not right. Tries left: %d.", "ka": "PIN-კოდი არასწორია. დარჩენილი ცდები: %d.", "ru": "Неверный PIN-код. Осталось попыток: %d."},
	"locked":              {"en": "Too many wrong tries. Please wait %d minutes and try again.", "ka": "ძალიან ბევრი არასწორი ცდა. მოიცადეთ %d წუთი და სცადეთ თავიდან.", "ru": "Слишком много неверных попыток. Подождите %d мин. и попробуйте снова."},
	"stale":               {"en": "This form is out of date. Please try again.", "ka": "ფორმა მოძველდა. გთხოვთ, სცადოთ თავიდან.", "ru": "Форма устарела. Попробуйте ещё раз."},
	"logout":              {"en": "Log out", "ka": "გასვლა", "ru": "Выйти"},
	"for_account":         {"en": "Settings for", "ka": "პარამეტრები ანგარიშისთვის:", "ru": "Настройки для"},
	"everyone":            {"en": "Everyone on this PC", "ka": "ამ კომპიუტერის ყველა მომხმარებელი", "ru": "Все пользователи этого компьютера"},
	"sec_limits":          {"en": "Age and daily time", "ka": "ასაკი და დღიური დრო", "ru": "Возраст и время в день"},
	"age":                 {"en": "Age", "ka": "ასაკი", "ru": "Возраст"},
	"years":               {"en": "%s years", "ka": "%s წელი", "ru": "%s лет"},
	"browse_min":          {"en": "Browsing time a day, in minutes", "ka": "ინტერნეტის დრო დღეში, წუთებში", "ru": "Время в интернете в день, в минутах"},
	"video_min":           {"en": "Video time a day, in minutes", "ka": "ვიდეოს დრო დღეში, წუთებში", "ru": "Время на видео в день, в минутах"},
	"limit_hint":          {"en": "Time on the learning sites in the bookmarks does not count.", "ka": "სანიშნეებში არსებულ სასწავლო საიტებზე გატარებული დრო არ ითვლება.", "ru": "Время на учебных сайтах из закладок не считается."},
	"bedtime":             {"en": "Bedtime", "ka": "ძილის დრო", "ru": "Время сна"},
	"starts":              {"en": "Starts", "ka": "იწყება", "ru": "Начало"},
	"ends":                {"en": "Ends", "ka": "მთავრდება", "ru": "Конец"},
	"focus":               {"en": "Focus mode: no feeds on YouTube and scroll sites", "ka": "ფოკუსის რეჟიმი: ლენტის გარეშე YouTube-სა და სოციალურ ქსელებში", "ru": "Режим фокуса: без ленты на YouTube и в соцсетях"},
	"focus_note":          {"en": "Shows a search box and videos from chosen learning channels instead of the feeds, also on the new tab page. The videos are loaded from YouTube.", "ka": "ლენტის ნაცვლად აჩვენებს საძიებო ველს და შერჩეული საგანმანათლებლო არხების ვიდეოებს, ახალი ჩანართის გვერდზეც. ვიდეოები YouTube-იდან იტვირთება.", "ru": "Вместо ленты показывает поле поиска и видео выбранных обучающих каналов, также на странице новой вкладки. Видео загружаются с YouTube."},
	"adblock":             {"en": "Ad blocker", "ka": "რეკლამის ბლოკირება", "ru": "Блокировка рекламы"},
	"family_dns":          {"en": "Family DNS filter (AdGuard DNS Family)", "ka": "საოჯახო DNS-ფილტრი (AdGuard DNS Family)", "ru": "Семейный DNS-фильтр (AdGuard DNS Family)"},
	"family_dns_note":     {"en": "Blocks adult sites, phishing and many ads. AdGuard DNS, a free public service, sees the names of the sites your child opens and your internet address, as any DNS service does. If it cannot be reached, sites do not open until it is back or you switch it off here. Clear the box to use your internet provider's DNS; the extension's own lists still work.", "ka": "ბლოკავს ზრდასრულთა საიტებს, ფიშინგს და რეკლამის დიდ ნაწილს. AdGuard DNS, უფასო საჯარო სერვისი, ხედავს იმ საიტების სახელებს, რომლებსაც ბავშვი ხსნის, და თქვენს ინტერნეტ-მისამართს, როგორც ნებისმიერი DNS-სერვისი. თუ ის მიუწვდომელია, საიტები არ გაიხსნება, სანამ ის არ აღდგება ან აქ არ გამორთავთ ფილტრს. მონიშვნა მოხსენით, რომ ინტერნეტპროვაიდერის DNS გამოიყენოთ; გაფართოების საკუთარი სიები მაინც იმუშავებს.", "ru": "Блокирует сайты для взрослых, фишинг и много рекламы. AdGuard DNS, бесплатный публичный сервис, видит названия сайтов, которые открывает ребёнок, и ваш интернет-адрес, как любой DNS-сервис. Если он недоступен, сайты не откроются, пока он не заработает или вы не выключите фильтр здесь. Снимите галочку, чтобы пользоваться DNS интернет-провайдера; собственные списки расширения продолжат работать."},
	"allow":               {"en": "Always allowed sites, one per line", "ka": "ყოველთვის დაშვებული საიტები, თითო ხაზზე ერთი", "ru": "Всегда разрешённые сайты, по одному в строке"},
	"block":               {"en": "Blocked sites, one per line", "ka": "დაბლოკილი საიტები, თითო ხაზზე ერთი", "ru": "Заблокированные сайты, по одному в строке"},
	"save":                {"en": "Save", "ka": "შენახვა", "ru": "Сохранить"},
	"defaults":            {"en": "Use the recommended times for this age", "ka": "ამ ასაკისთვის რეკომენდებული დროის დაყენება", "ru": "Поставить рекомендуемое время для этого возраста"},
	"saved":               {"en": "Saved. It applies now.", "ka": "შენახულია და უკვე მოქმედებს.", "ru": "Сохранено и уже действует."},
	"bad_input":           {"en": "Please check the values: nothing was saved.", "ka": "გთხოვთ, შეამოწმოთ მნიშვნელობები: არაფერი შენახულა.", "ru": "Проверьте значения: ничего не сохранено."},
	"sec_report":          {"en": "Last 7 days", "ka": "ბოლო 7 დღე", "ru": "Последние 7 дней"},
	"col_day":             {"en": "Day", "ka": "დღე", "ru": "День"},
	"col_browse":          {"en": "Browsing", "ka": "ინტერნეტი", "ru": "Интернет"},
	"col_video":           {"en": "Video", "ka": "ვიდეო", "ru": "Видео"},
	"col_learn":           {"en": "Learning", "ka": "სწავლა", "ru": "Учёба"},
	"col_blocked":         {"en": "Blocked pages", "ka": "დაბლოკილი გვერდები", "ru": "Заблокировано страниц"},
	"min":                 {"en": "min", "ka": "წთ", "ru": "мин"},
	"top_sites":           {"en": "Most-used sites", "ka": "ყველაზე ხშირად გამოყენებული საიტები", "ru": "Самые посещаемые сайты"},
	"no_data":             {"en": "Nothing yet.", "ka": "ჯერ არაფერია.", "ru": "Пока ничего нет."},
	"sec_browsers":        {"en": "Browsers on this PC", "ka": "ბრაუზერები ამ კომპიუტერზე", "ru": "Браузеры на этом компьютере"},
	"no_browsers":         {"en": "No browser found yet. The list updates every 5 minutes.", "ka": "ბრაუზერი ჯერ ვერ მოიძებნა. სია ყოველ 5 წუთში ახლდება.", "ru": "Браузеры пока не найдены. Список обновляется каждые 5 минут."},
	"sec_accounts":        {"en": "Protected Windows accounts", "ka": "დაცული Windows-ანგარიშები", "ru": "Защищённые учётные записи Windows"},
	"admin_note":          {"en": "administrator: can remove the protection", "ka": "ადმინისტრატორი: შეუძლია დაცვის მოხსნა", "ru": "администратор: может снять защиту"},
	"everyone_note":       {"en": "Only if the child has no own account. Time limits then apply to everyone, you too.", "ka": "მხოლოდ მაშინ, თუ ბავშვს საკუთარი ანგარიში არ აქვს. ამ შემთხვევაში დროის ლიმიტი ყველაზე მოქმედებს, თქვენზეც.", "ru": "Только если у ребёнка нет своей учётной записи. Тогда ограничения действуют на всех, и на вас тоже."},
	"no_accounts":         {"en": "Choose at least one account.", "ka": "აირჩიეთ ერთი ანგარიში მაინც.", "ru": "Выберите хотя бы одну учётную запись."},
	"sec_pin":             {"en": "Change the PIN", "ka": "PIN-კოდის შეცვლა", "ru": "Сменить PIN-код"},
	"pin_now":             {"en": "Current PIN", "ka": "ამჟამინდელი PIN-კოდი", "ru": "Текущий PIN-код"},
	"pin_new":             {"en": "New PIN, 4 to 8 digits", "ka": "ახალი PIN-კოდი, 4-დან 8 ციფრამდე", "ru": "Новый PIN-код, от 4 до 8 цифр"},
	"pin_again":           {"en": "New PIN again", "ka": "გაიმეორეთ ახალი PIN-კოდი", "ru": "Повторите новый PIN-код"},
	"pin_changed":         {"en": "The PIN is changed.", "ka": "PIN-კოდი შეიცვალა.", "ru": "PIN-код изменён."},
	"pin_mismatch":        {"en": "The new PINs are not the same, or not 4 to 8 digits.", "ka": "ახალი PIN-კოდები არ ემთხვევა ან არ შედგება 4-8 ციფრისგან.", "ru": "Новые PIN-коды не совпадают или не состоят из 4–8 цифр."},
	"honest":              {"en": "No filter is perfect: stay close and talk with your child about what they see online. The child needs a standard Windows account; an administrator can remove this protection.", "ka": "არცერთი ფილტრი არ არის სრულყოფილი: იყავით ახლოს და ესაუბრეთ ბავშვს იმაზე, რასაც ინტერნეტში ხედავს. ბავშვს სჭირდება Windows-ის სტანდარტული ანგარიში; ადმინისტრატორს შეუძლია ამ დაცვის მოხსნა.", "ru": "Ни один фильтр не идеален: будьте рядом и говорите с ребёнком о том, что он видит в интернете. Ребёнку нужна стандартная учётная запись Windows; администратор может снять эту защиту."},
	"by_reason":           {"en": "Blocked pages by reason", "ka": "დაბლოკილი გვერდები მიზეზის მიხედვით", "ru": "Заблокированные страницы по причинам"},
	"reason_adult":        {"en": "Adult content", "ka": "ზრდასრულთა კონტენტი", "ru": "Контент для взрослых"},
	"reason_gambling":     {"en": "Gambling", "ka": "აზარტული თამაშები", "ru": "Азартные игры"},
	"reason_bypass":       {"en": "Ways around the filter (proxies, VPNs)", "ka": "ფილტრის გვერდის ავლის საშუალებები (პროქსი, VPN)", "ru": "Обход фильтра (прокси, VPN)"},
	"reason_social":       {"en": "Social networks", "ka": "სოციალური ქსელები", "ru": "Соцсети"},
	"reason_topic":        {"en": "Sensitive topics", "ka": "მგრძნობიარე თემები", "ru": "Деликатные темы"},
	"topics":              {"en": "Sensitive topics", "ka": "მგრძნობიარე თემები", "ru": "Деликатные темы"},
	"topics_note":         {"en": "Blocks searches, pages and videos about sexual orientation and gender identity, yoga and meditation practices, horoscopes, tarot and witchcraft, Halloween and horror. Helplines stay open.", "ka": "ბლოკავს ძიებებს, გვერდებსა და ვიდეოებს სექსუალური ორიენტაციისა და გენდერული იდენტობის, იოგისა და მედიტაციის პრაქტიკების, ჰოროსკოპების, ტაროსა და ჯადოქრობის, ჰელოუინისა და საშინელებათა შესახებ. დახმარების ცხელი ხაზები ღია რჩება.", "ru": "Блокирует поиски, страницы и видео о сексуальной ориентации и гендерной идентичности, йоге и медитативных практиках, гороскопах, таро и колдовстве, Хэллоуине и ужасах. Телефоны доверия остаются открытыми."},
	"reason_content":      {"en": "Page content not right for the age", "ka": "ასაკისთვის შეუფერებელი შინაარსი", "ru": "Содержимое не по возрасту"},
	"reason_video":        {"en": "Videos", "ka": "ვიდეოები", "ru": "Видео"},
	"reason_time":         {"en": "Daily time used up", "ka": "დღიური დრო ამოიწურა", "ru": "Время на день закончилось"},
	"reason_bedtime":      {"en": "Bedtime", "ka": "ძილის დრო", "ru": "Время сна"},
	"reason_parent":       {"en": "Blocked by you", "ka": "თქვენ მიერ დაბლოკილი", "ru": "Заблокировано вами"},
	"st_active":           {"en": "Active · extension seen %s", "ka": "აქტიურია · გაფართოება ბოლოს ჩანდა %s", "ru": "Работает · расширение было активно %s"},
	"st_set":              {"en": "Settings set · extension not active yet", "ka": "პარამეტრები დაყენებულია · გაფართოება ჯერ არ მუშაობს", "ru": "Настройки заданы · расширение пока не работает"},
	"st_waiting":          {"en": "Settings follow at the child's next sign-in", "ka": "პარამეტრები დაყენდება ბავშვის შემდეგი შესვლისას", "ru": "Настройки применятся при следующем входе ребёнка"},
	"st_closed":           {"en": "Closed · not supported", "ka": "იხურება · არ არის მხარდაჭერილი", "ru": "Закрывается · не поддерживается"},
	"ago_now":             {"en": "just now", "ka": "ახლახან", "ru": "только что"},
	"ago_min":             {"en": "%d min ago", "ka": "%d წუთის წინ", "ru": "%d мин назад"},
	"tz":                  {"en": "Time zone", "ka": "სასაათო სარტყელი", "ru": "Часовой пояс"},
	"tz_note":             {"en": "Summer time is not followed: change this when the clocks change.", "ka": "ზაფხულის დრო ავტომატურად არ იცვლება: შეცვალეთ ეს, როცა საათი გადაიწევს.", "ru": "Летнее время не учитывается: измените это, когда переводят часы."},
	"pin_today":           {"en": "%d wrong PIN tries today, last at %s.", "ka": "დღეს PIN-კოდი %d-ჯერ შეიყვანეს არასწორად, ბოლოს %s-ზე.", "ru": "Сегодня неверный PIN-код вводили %d раз, последний раз в %s."},
	"sec_privacy":         {"en": "What is kept and who sees it", "ka": "რა ინახება და ვინ ხედავს", "ru": "Что хранится и кто это видит"},
	"privacy_here":        {"en": "What stays on this PC: minutes per day, the names of sites visited (no full addresses), blocked pages, search words (unless you switch them off), game minutes and closed apps, for 30 days, and your child's requests, for 7 days. Nothing is sent to RaiseSteady.", "ka": "რა რჩება ამ კომპიუტერზე: დღიური წუთები, ნანახი საიტების სახელები (სრული მისამართების გარეშე), დაბლოკილი გვერდები, საძიებო სიტყვები (თუ არ გამორთავთ), თამაშის წუთები და დახურული პროგრამები, 30 დღის განმავლობაში, ხოლო ბავშვის მოთხოვნები — 7 დღე. RaiseSteady-ს არაფერი ეგზავნება.", "ru": "Что остаётся на этом компьютере: минуты за день, названия посещённых сайтов (без полных адресов), заблокированные страницы, поисковые слова (если вы их не выключите), минуты игр и закрытые программы — 30 дней, а запросы ребёнка — 7 дней. В RaiseSteady ничего не отправляется."},
	"privacy_out":         {"en": "What leaves this PC: while the family DNS filter is on, your child's browsers look up site names through AdGuard DNS Family (a public filter); while focus mode is on, the chosen videos are loaded from YouTube.", "ka": "რა იგზავნება კომპიუტერის გარეთ: როცა საოჯახო DNS-ფილტრი ჩართულია, თქვენი შვილის ბრაუზერები საიტების სახელებს AdGuard DNS Family-ით (საჯარო ფილტრით) ამოწმებს; როცა ფოკუსის რეჟიმი ჩართულია, შერჩეული ვიდეოები YouTube-იდან იტვირთება.", "ru": "Что отправляется за пределы компьютера: пока включён семейный DNS-фильтр, браузеры ребёнка проверяют названия сайтов через AdGuard DNS Family (публичный фильтр); пока включён режим фокуса, подобранные видео загружаются с YouTube."},
	"browsers_only":       {"en": "It protects web browsers (Chrome, Edge, Brave, Firefox) and the PC games and chat apps in the list above. Other browsers are closed in your child's account. Other programs are not covered.", "ka": "იცავს ვებბრაუზერებს (Chrome, Edge, Brave, Firefox) და ზემოთ ჩამოთვლილ კომპიუტერის თამაშებსა და ჩატის პროგრამებს. სხვა ბრაუზერები თქვენი შვილის ანგარიშში იხურება. სხვა პროგრამები არ იფარება.", "ru": "Защищает веб-браузеры (Chrome, Edge, Brave, Firefox) и игры и чаты из списка выше. Другие браузеры закрываются в учётной записи ребёнка. Другие программы не охватываются."},
	"sec_accounts_linux":  {"en": "Accounts", "ka": "ანგარიშები", "ru": "Учётные записи"},
	"linux_all":           {"en": "On Linux, browsers take these settings for every account on this computer, including yours.", "ka": "Linux-ზე ბრაუზერები ამ პარამეტრებს ამ კომპიუტერის ყველა ანგარიშზე იყენებენ, თქვენის ჩათვლით.", "ru": "В Linux браузеры применяют эти настройки ко всем учётным записям этого компьютера, включая вашу."},
	"honest_linux":        {"en": "No filter is perfect: stay close and talk with your child about what they see online. Your child must not use an administrator account (one that can use sudo): an administrator can remove this protection.", "ka": "არცერთი ფილტრი არ არის სრულყოფილი: იყავით ახლოს და ესაუბრეთ ბავშვს იმაზე, რასაც ინტერნეტში ხედავს. ბავშვმა არ უნდა გამოიყენოს ადმინისტრატორის ანგარიში (რომელსაც sudo შეუძლია): ადმინისტრატორს შეუძლია ამ დაცვის მოხსნა.", "ru": "Ни один фильтр не идеален: будьте рядом и говорите с ребёнком о том, что он видит в интернете. Ребёнок не должен пользоваться учётной записью администратора (с правом sudo): администратор может снять эту защиту."},
	"browsers_only_linux": {"en": "It protects web browsers (Chrome, Chromium, Edge, Brave, Yandex Browser, Firefox) and the PC games and chat apps in the list above that have a Linux version. Other browsers, and other copies of these, are closed on this computer. Other programs are not covered.", "ka": "იცავს ვებბრაუზერებს (Chrome, Chromium, Edge, Brave, Yandex Browser, Firefox) და ზემოთ ჩამოთვლილ თამაშებსა და ჩატის პროგრამებს, რომლებსაც Linux-ის ვერსია აქვთ. სხვა ბრაუზერები და ამ ბრაუზერების სხვა ასლები ამ კომპიუტერზე იხურება. სხვა პროგრამები დაცვაში არ შედის.", "ru": "Защищает веб-браузеры (Chrome, Chromium, Edge, Brave, Яндекс Браузер, Firefox) и игры и чаты из списка выше, у которых есть версия для Linux. Другие браузеры и другие копии этих браузеров на этом компьютере закрываются. Другие программы не охватываются."},
	"sec_requests":        {"en": "Requests from your child", "ka": "ბავშვის მოთხოვნები", "ru": "Запросы ребёнка"},
	"your_child":          {"en": "Your child", "ka": "თქვენი შვილი", "ru": "Ваш ребёнок"},
	"req_site":            {"en": "%s asks you to allow %s.", "ka": "%s ითხოვს, დაუშვათ %s.", "ru": "%s просит разрешить %s."},
	"req_time":            {"en": "%s asks for more time today.", "ka": "%s ითხოვს დამატებით დროს დღეისთვის.", "ru": "%s просит ещё времени на сегодня."},
	"req_note":            {"en": "Note: “%s”", "ka": "შენიშვნა: „%s“", "ru": "Записка: «%s»"},
	"req_at":              {"en": "Asked %s", "ka": "მოთხოვნის დრო: %s", "ru": "Время запроса: %s"},
	"req_allow":           {"en": "Allow this site", "ka": "საიტის დაშვება", "ru": "Разрешить этот сайт"},
	"req_no":              {"en": "No", "ka": "არა", "ru": "Нет"},
	"req_hint":            {"en": "Requests stay here for 7 days. Your child sees no answer here, only the result: the site opens, or more time is added.", "ka": "მოთხოვნები აქ 7 დღე ინახება. ბავშვი პასუხს არ ხედავს, მხოლოდ შედეგს: საიტი იხსნება ან დრო ემატება.", "ru": "Запросы хранятся здесь 7 дней. Ребёнок не видит ответа, только результат: сайт открывается или времени становится больше."},
	"req_done":            {"en": "Done. Your answer applies within a minute.", "ka": "მზადაა. პასუხი ერთ წუთში ამოქმედდება.", "ru": "Готово. Ответ подействует в течение минуты."},
	"req_gone":            {"en": "That request was already answered.", "ka": "ამ მოთხოვნას უკვე უპასუხეს.", "ru": "На этот запрос уже ответили."},
	"n_min":               {"en": "%d min", "ka": "%d წთ", "ru": "%d мин"},
	"n_hours":             {"en": "%d h", "ka": "%d სთ", "ru": "%d ч"},
	"sec_pause":           {"en": "Pause the internet", "ka": "ინტერნეტის შეჩერება", "ru": "Приостановить интернет"},
	"pause_note":          {"en": "Every site then shows a calm page saying it is paused, and PC games on game time are closed. End it early here, or with the PIN on that page.", "ka": "ამ დროს ყველა საიტზე ჩანს მშვიდი გვერდი, რომ ინტერნეტი შეჩერებულია, და თამაშის დროზე მყოფი თამაშები იხურება. ადრე დასრულება შეგიძლიათ აქ ან PIN-კოდით იმავე გვერდზე.", "ru": "Тогда на каждом сайте видна спокойная страница о паузе, а игры с режимом «Время для игр» закрываются. Закончить раньше можно здесь или PIN-кодом на той странице."},
	"pause_tomorrow":      {"en": "Until 06:00 tomorrow", "ka": "ხვალ 06:00-მდე", "ru": "До 06:00 завтра"},
	"pause_on":            {"en": "Paused until %s.", "ka": "შეჩერებულია %s-მდე.", "ru": "Приостановлено до %s."},
	"pause_off":           {"en": "Not paused.", "ka": "არ არის შეჩერებული.", "ru": "Сейчас не приостановлено."},
	"resume":              {"en": "Resume now", "ka": "ახლავე განახლება", "ru": "Возобновить сейчас"},
	"pause_saved":         {"en": "Done. Browsers follow within a minute.", "ka": "მზადაა. ბრაუზერებში ერთ წუთში ამოქმედდება.", "ru": "Готово. В браузерах это подействует в течение минуты."},
	"weekend":             {"en": "Weekend", "ka": "შაბათ-კვირა", "ru": "Выходные"},
	"weekend_on":          {"en": "Other times at the weekend", "ka": "შაბათ-კვირას სხვა დრო", "ru": "Другое время в выходные"},
	"weekend_note":        {"en": "Saturday and Sunday use these minutes. This bedtime is for Friday and Saturday nights.", "ka": "შაბათსა და კვირას ეს წუთები მოქმედებს. ეს ძილის დრო პარასკევისა და შაბათის ღამისთვისაა.", "ru": "В субботу и воскресенье действуют эти минуты. Это время сна — для ночей с пятницы на субботу и с субботы на воскресенье."},
	"homework":            {"en": "Homework time", "ka": "საშინაო დავალების დრო", "ru": "Время для домашних заданий"},
	"homework_on":         {"en": "Homework time on school days (Monday to Friday)", "ka": "საშინაო დავალების დრო სასწავლო დღეებში (ორშაბათიდან პარასკევამდე)", "ru": "Время для домашних заданий в учебные дни (с понедельника по пятницу)"},
	"homework_note":       {"en": "In this time only the learning sites in the bookmarks and your always-allowed sites open, and PC games on game time are closed.", "ka": "ამ დროს იხსნება მხოლოდ სანიშნეებში არსებული სასწავლო საიტები და თქვენ მიერ ყოველთვის დაშვებული საიტები, ხოლო თამაშის დროზე მყოფი თამაშები იხურება.", "ru": "В это время открываются только учебные сайты из закладок и всегда разрешённые вами сайты, а игры с режимом «Время для игр» закрываются."},
	"searches":            {"en": "Show the searches in the report", "ka": "ძიებების ჩვენება ანგარიშში", "ru": "Показывать поисковые запросы в отчёте"},
	"searches_note":       {"en": "The words your child searches for on Google, Bing, DuckDuckGo, Yahoo, Ecosia, Yandex and YouTube, up to 50 a day, kept on this PC for 30 days. The extension tells your child.", "ka": "სიტყვები, რომლებსაც ბავშვი ეძებს Google-ში, Bing-ში, DuckDuckGo-ში, Yahoo-ში, Ecosia-ში, Yandex-სა და YouTube-ზე, დღეში 50-მდე, ამ კომპიუტერზე 30 დღე ინახება. გაფართოება ამის შესახებ ბავშვს ეუბნება.", "ru": "Слова, которые ребёнок ищет в Google, Bing, DuckDuckGo, Yahoo, Ecosia, Яндексе и на YouTube, до 50 в день, хранятся на этом компьютере 30 дней. Расширение сообщает об этом ребёнку."},
	"sec_apps":            {"en": "PC games and apps", "ka": "კომპიუტერის თამაშები და პროგრამები", "ru": "Игры и программы на компьютере"},
	"game_min":            {"en": "Game time a day, in minutes", "ka": "თამაშის დრო დღეში, წუთებში", "ru": "Время для игр в день, в минутах"},
	"apps_note":           {"en": "Game time counts while any app set to Game time is open, also minimized. When it is used up, at bedtime, in homework time and while the internet is paused, those apps are closed. Blocked apps are always closed. Closing Steam does not always close a game started from it. Only these apps are covered; other programs are not.", "ka": "თამაშის დრო ითვლება, სანამ „თამაშის დროზე“ დაყენებული რომელიმე პროგრამა გახსნილია, ჩაკეცილიც. როცა დრო ამოიწურება, ძილის დროს, საშინაო დავალების დროს და როცა ინტერნეტი შეჩერებულია, ეს პროგრამები იხურება. დაბლოკილი პროგრამები ყოველთვის იხურება. Steam-ის დახურვა ყოველთვის არ ხურავს მისით გაშვებულ თამაშს. იფარება მხოლოდ ეს პროგრამები; სხვები არა.", "ru": "Время для игр считается, пока открыта любая программа с режимом «Время для игр», даже свёрнутая. Когда оно закончилось, во время сна, во время домашних заданий и на паузе такие программы закрываются. Заблокированные программы закрываются всегда. Закрытие Steam не всегда закрывает игру, запущенную из него. Охватываются только эти программы, другие — нет."},
	"app_allowed":         {"en": "Allowed", "ka": "დაშვებული", "ru": "Разрешено"},
	"app_game":            {"en": "Game time", "ka": "თამაშის დრო", "ru": "Время для игр"},
	"app_blocked":         {"en": "Blocked", "ka": "დაბლოკილი", "ru": "Заблокировано"},
	"app_min_age":         {"en": "Minimum age in its terms: %d", "ka": "მინიმალური ასაკი მისი წესების მიხედვით: %d", "ru": "Минимальный возраст по его правилам: %d"},
	"col_games":           {"en": "PC games", "ka": "თამაშები", "ru": "Игры"},
	"day_pick":            {"en": "Pick a day to see its searches and closed apps.", "ka": "აირჩიეთ დღე, რომ ნახოთ მისი ძიებები და დახურული პროგრამები.", "ru": "Выберите день, чтобы увидеть его поиски и закрытые программы."},
	"searches_day":        {"en": "Searches on %s", "ka": "ძიებები, %s", "ru": "Поиски за %s"},
	"searches_off":        {"en": "Searches are not recorded: the switch above is off.", "ka": "ძიებები არ ინახება: ზემოთ ეს ფუნქცია გამორთულია.", "ru": "Поиски не записываются: выше это выключено."},
	"apps_closed_day":     {"en": "PC apps closed on %s", "ka": "დახურული პროგრამები, %s", "ru": "Закрытые программы за %s"},
	"reason_homework":     {"en": "Homework time", "ka": "საშინაო დავალების დრო", "ru": "Время для домашних заданий"},
	"reason_paused":       {"en": "Internet paused by you", "ka": "თქვენ მიერ შეჩერებული ინტერნეტი", "ru": "Интернет приостановлен вами"},
	"game_extra_note":     {"en": "Extra minutes you give, with the PIN or as an answer to a request, count for games too.", "ka": "დამატებითი წუთები, რომლებსაც PIN-კოდით ან მოთხოვნაზე პასუხად აძლევთ, თამაშებზეც ვრცელდება.", "ru": "Дополнительные минуты, которые вы даёте PIN-кодом или в ответ на запрос, действуют и для игр."},
	"all_accounts_apps":   {"en": "In this mode the PC apps, the pause and the searches apply to every account, yours too.", "ka": "ამ რეჟიმში კომპიუტერის პროგრამები, შეჩერება და ძიებები ყველა ანგარიშზე მოქმედებს, თქვენზეც.", "ru": "В этом режиме программы, пауза и поиски действуют для всех учётных записей, и для вашей тоже."},
	"language":            {"en": "Language", "ka": "ენა", "ru": "Язык"},

	// News from RaiseSteady (P2-252); the copy is fixed in the P2-252 brief.
	"news_title": {"en": "News from RaiseSteady", "ka": "სიახლეები RaiseSteady-სგან", "ru": "Новости от RaiseSteady"},
	"news_ask":   {"en": "Show short news from the makers of RaiseSteady Guard on this page: new versions, fixes, tips.", "ka": "ამ გვერდზე მოკლე სიახლეების ჩვენება RaiseSteady Guard-ის შემქმნელებისგან: ახალი ვერსიები, გასწორებები, რჩევები.", "ru": "Показывать на этой странице короткие новости от создателей RaiseSteady Guard: новые версии, исправления, советы."},
	"news_how":   {"en": "When this page opens, it reads a small news file from raisesteady.com. Nothing about you or your child is sent. Your internet address reaches the website, as with any website; we keep no record of it.", "ka": "როცა ეს გვერდი იხსნება, ის raisesteady.com-დან კითხულობს პატარა ფაილს სიახლეებით. თქვენზე ან თქვენს შვილზე არაფერი იგზავნება. თქვენი ინტერნეტ-მისამართი საიტამდე აღწევს, როგორც ნებისმიერ საიტზე; ჩვენ მას არ ვინახავთ.", "ru": "Когда эта страница открывается, она читает небольшой файл с новостями с raisesteady.com. О вас и вашем ребёнке ничего не передаётся. Ваш интернет-адрес доходит до сайта, как с любым сайтом; мы его не сохраняем."},
	"news_on":    {"en": "Turn on", "ka": "ჩართვა", "ru": "Включить"},
	"news_off":   {"en": "Turn off news", "ka": "სიახლეების გამორთვა", "ru": "Выключить новости"},
	"news_hide":  {"en": "Hide", "ka": "დამალვა", "ru": "Скрыть"},
	"news_more":  {"en": "Read more", "ka": "მეტის წაკითხვა", "ru": "Подробнее"},
	// The self-updater (P2-260): one calm line, no switch.
	"upd_line":    {"en": "Version %s. Last check for updates: %s. Last update: %s.", "ka": "ვერსია %s. განახლებების ბოლო შემოწმება: %s. ბოლო განახლება: %s.", "ru": "Версия %s. Последняя проверка обновлений: %s. Последнее обновление: %s."},
	"upd_never":   {"en": "not yet", "ka": "ჯერ არა", "ru": "ещё не было"},
	"upd_failing": {"en": "The last checks for updates did not succeed. Guard keeps working as it is and tries again later.", "ka": "განახლებების ბოლო შემოწმებები ვერ შესრულდა. Guard ჩვეულებრივ აგრძელებს მუშაობას და მოგვიანებით ისევ სცდის.", "ru": "Последние проверки обновлений не удались. Guard продолжает работать как прежде и попробует снова позже."},
	"news_setup":  {"en": "News from RaiseSteady on the parent page (reads a small file from raisesteady.com when the page opens)", "ka": "სიახლეები RaiseSteady-სგან მშობლის გვერდზე (გვერდის გახსნისას raisesteady.com-დან პატარა ფაილს კითხულობს)", "ru": "Новости от RaiseSteady на странице родителя (при открытии страница читает небольшой файл с raisesteady.com)"},
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
