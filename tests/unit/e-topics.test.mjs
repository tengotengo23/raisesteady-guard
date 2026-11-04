// Topic filter (P2-161, T1): topic texts in en, ka and ru are found; ordinary texts with look-alike words
// are not; the score rule for descriptions and headings; helplines are never blocked; the domain list.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const src = readFileSync(new URL('../../ext/content/topics.js', import.meta.url), 'utf8');
const data = JSON.parse(readFileSync(new URL('../../shared/topics.json', import.meta.url), 'utf8'));
const sandbox = {};
sandbox.globalThis = sandbox;
vm.runInNewContext(src, sandbox);
const T = sandbox.KGTopics.create(data);

const TOPIC = [
  // review M3, M4, S2: compounds, Russian trans with a space, self-discovery
  'gaycouple', 'gay kiss', 'транс люди', 'транс девушка', 'транс-девушка', 'транс подросток', 'транс сообщество',
  'am i trans', 'am i gay', 'am i bi', 'what gender am i', "what's my sexuality", 'questioning my gender', 'gender quiz', 'sexuality test', 'ftm transition', 'mtf surgery', 'how do i know if i am gay',
  'тест на ориентацию', 'тест на сексуальную ориентацию', 'какой у меня гендер', 'как понять что я би', 'как понять, что я лесбиянка', 'я гей или нет',
  'ვარ თუ არა ბისექსუალი', 'რა გენდერის ვარ', 'ორიენტაციის ტესტი', 'როგორ გავიგო ვარ თუ არა გეი', 'gey', 'lezbian',
  // searches and titles, English
  'lgbt', 'LGBTQ+ youth', 'lgbtqia meaning', '2SLGBTQ+ resources', 'what does queer mean', 'Transgender kids', 'trans women in sport',
  'gender identity quiz', 'am i non-binary', 'Nonbinary flag colours', 'genderfluid', 'gender transition for teens', 'my pronouns are they/them',
  'they/them pronouns explained', 'coming out as gay', 'Coming out to my parents', 'how to come out as bi', 'Pride Month 2026', 'pride parade london',
  'drag queen story hour', 'RuPaul', 'same-sex marriage', 'gay', 'Am I gay?', 'lesbian couple', 'bisexual', 'pansexual meaning', 'asexual meaning',
  'sexual orientation test', 'puberty blockers', 'gender dysphoria in children', 'Is my child transgender?', 'deadnaming', 'sex change surgery',
  // Georgian
  'ლგბტ', 'ლგბტქ+ თემი', 'ქვირ ხელოვნება', 'ტრანსგენდერი ბავშვები', 'გენდერული იდენტობა', 'რა არის გენდერული იდენტობის დარღვევა', 'გეი წყვილი',
  'ლესბოსელი', 'ბისექსუალი', 'სექსუალური ორიენტაცია', 'თბილისი პრაიდი', 'არაბინარული ადამიანი', 'ქამინგ აუთი', 'ერთსქესიანი ქორწინება', 'სქესის შეცვლა',
  // Russian
  'лгбт', 'ЛГБТ-сообщество', 'квир культура', 'Трансгендеры', 'гендерная идентичность', 'небинарный человек', 'гей', 'геи в истории', 'гейская пара',
  'лесбиянки', 'бисексуальность', 'сексуальная ориентация', 'каминг-аут', 'гей-парад', 'однополые браки', 'смена пола', 'нетрадиционная ориентация',
];

const NOT_TOPIC = [
  // surnames, history, botany, classic books (review S3, S4, N1)
  'Tyson Gay 100m', "John Gay, The Beggar's Opera", 'Peter Gay historian', 'the gay nineties', 'bisexual flowers', 'Bisexual and unisexual flowers', 'бисексуальные цветки', 'ბისექსუალური ყვავილი',
  'The Secret Garden: a queer little man', 'a queer feeling in my tummy', 'noun gender quiz French', 'baby gender test', 'FTM crypto price', 'MTF trading strategy', 'school orientation test',
  'транс музыка', 'транс диджей', 'трансперсональная психология', 'транспарант', 'Lesbos island holiday',
  // English ordinary texts with look-alike words
  "Gay-Lussac's law", 'Gay Lussac law for kids', 'transport', 'Trans-Siberian Railway', 'transformers', 'transition words for kids', 'translation',
  'Pride and Prejudice', 'pride of lions', 'proud and pride poem', 'coming out of the cave', 'the cake came out as a disaster', 'Enola Gay history',
  'Deck the Halls: don we now our gay apparel', 'asexual reproduction in plants', 'asexual and sexual reproduction', 'drag and drop', 'drag race cars',
  'subject pronouns he she they', 'binary numbers', 'non-binary trees in data structures', 'gender of nouns in French', 'clownfish change their sex',
  'sex change in fish', 'Queen Elizabeth', 'gaming', 'Gaylord the dog', 'coming out of hibernation', 'homework help', 'transgenic plants',
  // Georgian ordinary words
  'გეიზერი', 'გეიმერი', 'გეიმინგი', 'გეიმი ონლაინ', 'ტრანსპორტი', 'ტრანსფორმერები', 'ქვეყანა', 'პრაქტიკა', 'გენდერი ბიოლოგიაში არ არის', 'მცენარეების ასექსუალური გამრავლება',
  'იდენტობა და ისტორია', 'ლექსი', 'ბიზნესი',
  // Russian ordinary words
  'трансформер', 'трансформеры игрушки', 'гейзер', 'гейзеры камчатки', 'геймер', 'гейм', 'геймпад', 'гейша', 'Билл Гейтс', 'прайд львов',
  'однополые цветки', 'бесполое размножение', 'асексуальное размножение растений', 'у меня пол холодный', 'рыбы меняют пол', 'транспорт', 'квиринал',
];

test('topic searches and titles are found (en, ka, ru)', () => {
  const missed = TOPIC.filter((s) => !T.matchQuery(s).hit);
  assert.deepEqual(missed, []);
});

test('ordinary texts with look-alike words pass (en, ka, ru)', () => {
  const wrong = NOT_TOPIC.filter((s) => T.matchQuery(s).hit).map((s) => `${s} -> ${T.matchQuery(s).terms}`);
  assert.deepEqual(wrong, []);
});

test('a page: title, og:title or the address path blocks', () => {
  assert.equal(T.matchPage({ host: 'news.example', path: '/', title: 'Coming out as transgender at school' }).why, 'title');
  assert.equal(T.matchPage({ host: 'news.example', path: '/', title: 'Story', ogTitle: 'Лесбиянки в кино' }).why, 'title');
  assert.equal(T.matchPage({ host: 'news.example', path: '/features/lgbt-history-month', title: 'Features' }).why, 'path');
  assert.equal(T.matchPage({ host: 'chem.example', path: '/gay-lussac-law', title: "Gay-Lussac's law" }).hit, false);
  assert.equal(T.matchPage({ host: 'travel.example', path: '/trans-siberian', title: 'Trans-Siberian Railway' }).hit, false);
});

test('score rule: one word in a long description does not block; two clear terms or two places do', () => {
  const one = T.matchPage({ host: 'news.example', path: '/world', title: 'World news today', description: 'Elections, sport, the weather and a court ruling on same-sex marriage in Europe.', headings: ['World', 'Sport'] });
  assert.equal(one.hit, false);
  assert.equal(one.score, 2);
  const two = T.matchPage({ host: 'blog.example', path: '/post/12', title: 'My story', description: 'Growing up bisexual and finding a queer community.' });
  assert.equal(two.hit, true);
  assert.equal(two.why, 'score');
  const places = T.matchPage({ host: 'blog.example', path: '/p', title: 'Notes', description: 'A guide to gender identity.', headings: ['What is gender identity?'] });
  assert.equal(places.hit, true);
  const weak = T.matchPage({ host: 'school.example', path: '/p', title: 'Anti-bullying week', description: 'Homophobic and transphobic bullying, racism and name-calling.' });
  assert.equal(weak.hit, false, 'weak words alone (bullying pages) do not block');
});

test('body text is never read', () => {
  const r = T.matchPage({ host: 'news.example', path: '/', title: 'Weather', text: 'lgbt lgbt lgbt transgender gay lesbian' });
  assert.equal(r.hit, false);
});

test('helplines are never blocked, whatever the page says', () => {
  for (const host of ['findahelpline.com', 'www.findahelpline.com', 'childhelplineinternational.org', 'www.samaritans.org', 'telefon-doveria.ru', 'www.thetrevorproject.org']) {
    const r = T.matchPage({ host, path: '/lgbtq-youth', title: 'Support for LGBTQ young people', description: 'gay, lesbian, bisexual and transgender' });
    assert.equal(r.hit, false, host);
    assert.equal(r.why, 'helpline');
    assert.equal(T.isHelpline(host), true);
    assert.equal(T.isTopicDomain(host), false);
  }
  assert.equal(T.helpLink, 'https://findahelpline.com/');
});

test('domain list: sites mainly about the topic, subdomains too; others not', () => {
  assert.equal(T.matchPage({ host: 'www.glaad.org', path: '/', title: 'Home' }).why, 'domain');
  assert.equal(T.isTopicDomain('news.tbilisipride.ge'), true);
  assert.equal(T.isTopicDomain('notglaad.org'), false);
  assert.equal(T.isTopicDomain('wikipedia.org'), false);
});

test('the data file: every pattern compiles, ids are unique, lists are domains', () => {
  assert.equal(T.size, data.terms.length);
  assert.equal(new Set(data.terms.map((t) => t[0])).size, data.terms.length);
  for (const d of [...data.domains, ...data.helplines]) assert.match(d, /^[a-z0-9.-]+\.[a-z]{2,}$/);
  assert.equal(data.domains.filter((d) => data.helplines.includes(d)).length, 0);
});

test('broken data never throws', () => {
  const t = sandbox.KGTopics.create({ terms: [['bad', '(', 2], 'x', null], domains: 'x' });
  assert.equal(t.matchQuery('lgbt').hit, false);
  assert.equal(sandbox.KGTopics.create(null).matchPage(null).hit, false);
});

// Spelling tricks a child might use (owner's rule, 4 Nov: fixed rules only, as tight as they can be).
const TRICKS = {
  'gaps between letters': { topic: ['l g b t', 'g a y', 'q u e e r stories', 'л г б т', 'გ ე ი'], plain: ['a b c song', 'g a m e', 't r a i n', 'B I N G O', 'I l o v e you'] },
  'dots or stars inside a word': { topic: ['l.g.b.t', 'g.a.y', 'tran.sgender', 'les*bian', 'bi-sexual', 'trans_gender', 'лгб.т'], plain: ['U.S.A.', 'e.g. examples', 'a.m. and p.m.', 'Mr. Gaylord', 'co-op games', 'x-ray'] },
  'digits and signs for letters': { topic: ['g4y', 'l3sbian', 'tr4nsgender', 'b1sexual', 'qu33r', 'lgbtq1a', '@sexual meaning', 'tr@ns kids', 'g@y'], plain: ['h2o', '4x4 trucks', 'mp3 player', 'covid-19', 'top 10 games', '1st grade', 'g4me', '3d printing', 'w3schools', 'b4 school'] },
  'Latin and Cyrillic look-alike letters': { topic: ['gаy', 'lеsbian', 'trаnsgender', 'гeй', 'лeсбиянки', 'тpaнсгендер'], plain: ['Москва', 'Привет', 'трансфoрмер', 'gаme', 'гeйзер', 'cтол'] },
  'repeated letters': { topic: ['gaaay', 'lgbtttt', 'queeeer'], plain: ['loooove', 'yesss', 'zzz sleep', 'gooood morning'] },
  'hashtags': { topic: ['#gayboy', '#gaycouple', '#gaykiss', '#gaystory', '#gayboys', '#gaypride', 'gaytiktok', '#lgbtq', '#pridemonth', '#PrideMonth', '#transrights', '#trans_rights', '#comingout', '#dragqueen', '#лгбт', '#ლგბტ'], plain: ['#prideandprejudice', '#transport', '#dragrace', '#throwback', '#gaming', '#gameday', '#transformers'] },
  'Georgian and Russian written in Latin letters': { topic: ['gei wyvili', 'gei kaci', 'lesboseli', 'biseksuali', 'genderuli identoba', 'seksualuri orientacia', 'lesbiyanka', 'gomoseksualist', 'kaming aut', 'odnopolye braki'], plain: ['geiger counter', 'gela', 'lesi', 'seksualuri ganatleba', 'genderuli tanasworoba skolashi', 'gei wo yi shou ge', 'wo gei ni', 'gei (给) pinyin'] },
  'invisible and full-width characters': { topic: ['g\u200ba\u200by', 'g\u200da\u200dy', 'l\u00adgbt', 'ｇａｙ', 'ＬＧＢＴ', 'л\u200bг\u200bб\u200bт', 'გ\u200bე\u200bი'], plain: ['ga\u200bme', 'tra\u200bin', 'ｇａｍｅ', 'Ｐｒｉｄｅ and Prejudice'] },
  'emoji between letters': { topic: ['g😀a😀y', 'l🌈g🌈b🌈t'], plain: ['g😀a😀m😀e', 'a 🌈 b 🌈 c'] },
  'digits after a word': { topic: ['lgbtqia2s+', 'gay2026', 'lgbt2'], plain: ['transport2026', 'gaming2'] },
};
for (const [trick, { topic, plain }] of Object.entries(TRICKS)) {
  test(`spelling trick: ${trick} (found, and look-alikes pass)`, () => {
    assert.deepEqual(topic.filter((s) => !T.matchQuery(s).hit), [], 'missed');
    assert.deepEqual(plain.filter((s) => T.matchQuery(s).hit), [], 'wrongly found');
  });
}
