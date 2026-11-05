// Values terms of the topic filter (P2-175): yoga and eastern meditation, the occult, horoscopes, Halloween,
// horror, gender choice; en ka ru es zh el sr. Ordinary texts with near words stay open. Kind 'values' vs 'topic'.
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

const VALUES = [
  // the feed titles that started this (P2-175)
  'Thanksgiving Yoga for Kids - Bongo the Bear | A Cosmic Kids Yoga Adventure!', 'How to Deal with Nightmares | Mindfulness for Kids | Cosmic Kids Zen Den',
  'Top 20 Halloween Songs 🎃 🍬| Kids Songs | Super Simple Songs', 'Work It Out Wombats! FULL EPISODE | Summerweenie Halloweenie 🎃☀️ | PBS KIDS',
  'Halloween Costume Dress Up with Mother Goose Club! 👻🎃 #halloween #trickortreat #dressup #scary', 'GoNoodle Halloween Day of Dance! | GoNoodleween For Kids of All Ages',
  'Moonzy 🎡 Water-go-round 🎃 New Halloween Episode', 'Ghostbusters Kids Yoga Adventure 👻 | Yoga for Kids | Cosmic Kids',
  // English searches and titles
  'kids yoga', 'yoga for beginners', 'Namaste', 'chakra balancing', 'guided meditation for kids', 'sleep meditation', 'mindfulness for children',
  'mindful breathing', 'om chanting', 'reiki for beginners', 'healing crystals', 'law of attraction', 'third eye opening',
  'witchcraft for beginners', 'how to become a witch', 'baby witch tips', 'love spells that work', 'how to cast a spell', 'spell jar', 'ouija board',
  'satanic ritual', 'tarot reading', 'daily horoscope', 'what is my zodiac sign', 'astrology explained', 'birth chart reading', 'angel numbers 111',
  'leo season', 'trick or treat', 'spooky season', 'jack o lantern carving', 'day of the dead', 'scary stories for kids', 'creepypasta', 'horror movie',
  'haunted house tour', 'ghost caught on camera', 'five nights at freddys', 'FNAF song', 'poppy playtime chapter 3', 'huggy wuggy', 'skibidi toilet',
  'zombie apocalypse', 'vampire diaries', 'werewolf transformation', 'scp-096', 'h a l l o w e e n', 'y.o.g.a for kids',
  // Russian
  'йога для детей', 'детская йога', 'медитация для сна', 'гороскоп на сегодня', 'знак зодиака', 'карты таро', 'хэллоуин костюмы', 'хеллоуин', 'гадание на картах',
  'приворот на любовь', 'страшилки для детей', 'хоррор игры', 'зомби апокалипсис', 'вампиры', 'фнаф', 'скибиди туалет', 'сатанизм', 'экстрасенсы',
  // Georgian
  'იოგა ბავშვებისთვის', 'მედიტაცია', 'ჰოროსკოპი', 'ჰელოუინი', 'ტარო', 'ასტროლოგია', 'ზომბი', 'ვამპირები', 'მკითხავი', 'ჯადოქრობა',
  // Spanish, Chinese, Greek, Serbian
  'meditación guiada', 'brujería', 'horóscopo de hoy', 'truco o trato', 'historias de terror', '儿童瑜伽', '冥想音乐', '万圣节服装', '塔罗牌', '星座运势', '鬼故事',
  'γιόγκα για παιδιά', 'χάλοουιν', 'ωροσκόπιο', 'αστρολογία', 'јога за децу', 'ноћ вештица', 'noć veštica', 'хороскоп',
];
const GENDER = ['choose your gender', 'there are more than two genders', '100 genders', 'the genderbread person', 'gender is a spectrum'];
const OPEN = [
  // near words and other meanings
  'yogurt recipe', 'Yogi Bear cartoon', 'йогурт', 'იოგურტი', 'how to spell cat', 'spelling test', 'cold spell in winter', 'a dry spell',
  'how to draw a cartoon cat', 'The Lion, the Witch and the Wardrobe', 'Hansel and Gretel', 'Баба-Яга', 'Морозко', 'Snow White and the seven dwarfs',
  'vampire bats drink blood', 'zombie ants and the cordyceps fungus', 'spooky action at a distance', 'the horrors of World War I', 'pentagram geometry golden ratio',
  'Chinese zodiac animals and the Great Race', 'Marcus Aurelius Meditations', 'Christian meditation on the psalms', 'prayer of the heart',
  'German has 3 genders: der die das', 'masculine and feminine nouns in French', 'Cookie Monster', 'Monster trucks for kids', 'magic tricks for kids',
  'The Magic Hat', 'manifesting symptoms of flu', 'pumpkin soup', 'autumn leaves', 'Lesbos island', 'Λέσβος', 'λεσβιακό ούζο',
  'Christmas carols', 'St Nicholas', 'Easter eggs', 'Numberblocks', 'How do bees make honey?', 'Leonardo da Vinci', 'Pisces constellation stars',
];

test('values titles and searches are found, kind "values"', () => {
  for (const s of VALUES) {
    const m = T.matchTitle(s);
    assert.ok(m.hit, `should block: ${s}`);
    assert.equal(m.kind, 'values', `kind for: ${s} (${m.terms})`);
  }
});

test('gender choice is found, kind "topic" (the gender wording, with helplines)', () => {
  for (const s of GENDER) { const m = T.matchTitle(s); assert.ok(m.hit, s); assert.equal(m.kind, 'topic', s); }
});

test('ordinary texts with near words stay open', () => {
  for (const s of OPEN) assert.equal(T.matchTitle(s).hit, false, `should stay open: ${s} (${T.matchTitle(s).terms})`);
});

test('mixed hits are kind "topic"; pages: values domain and score', () => {
  assert.equal(T.matchTitle('lgbt yoga').kind, 'topic');
  assert.equal(T.matchPage({ host: 'www.horoscope.com', path: '/' }).kind, 'values');
  assert.equal(T.matchPage({ host: 'glaad.org', path: '/' }).kind, 'topic');
  assert.ok(T.isValuesDomain('cafeastrology.com') && !T.isValuesDomain('glaad.org'));
  const p = T.matchPage({ host: 'example.org', path: '/x', title: 'Autumn fun', description: 'Halloween crafts and trick or treat ideas', headings: ['Halloween party'] });
  assert.ok(p.hit && p.kind === 'values', JSON.stringify(p));
});

test('valuesDomains are all in domains (Android reads domains); helplines are never values', () => {
  const all = new Set(data.domains);
  for (const d of data.valuesDomains) assert.ok(all.has(d), d);
  for (const h of data.helplines) assert.ok(!data.valuesDomains.includes(h), h);
});

test('every pattern compiles', () => {
  assert.equal(T.size, data.terms.length);
});

// P2-175 (the owner, 5 Nov 21:08): no sex, no porn, no nudity in what we recommend, nor on pages and searches.
test('sex, porn and nudity words are found in every language; biology of plants and animals stays open', () => {
  for (const s of ['Hi-Tech Sex (Full Episode) | Original Sin: Sex', 'sexy dance', 'sex education', 'porn', 'pornhub', 'nude beach', 'naked and afraid', 'erotic stories',
    'OnlyFans', 'strip club', 'lingerie haul', 'how to seduce', 'kissing scenes', 'animals mating', 'big boobs', '18+ video',
    'секс', 'порно', 'голая девушка', 'эротика', 'სექსი', 'პორნო', 'შიშველი', 'sexo', 'desnuda', '色情', '性感美女', 'σεξ', 'πορνό', 'seks', 'голи', 'seksualuri ganatleba']) {
    assert.ok(T.matchTitle(s).hit, `should block: ${s}`);
  }
  for (const s of ['sexual reproduction in plants', 'how flowers make seeds', 'temperature-dependent sex determination in turtles', 'Essex and Sussex', 'unisex names',
    'chicken breast recipe', 'breast cancer awareness', 'the sixth sense', 'γυμναστική για παιδιά', 'Middlesex University', 'gymnastics', 'Pisces constellation']) {
    assert.equal(T.matchTitle(s).hit, false, `should stay open: ${s} (${T.matchTitle(s).terms})`);
  }
});

test('feed-only dark themes: left out of our picks, pages stay open', () => {
  const F = sandbox.KGTopics.create(data, { feed: true });
  for (const s of ['Unraveling the Notorious Stanford Prison Experiment', 'AI Psychosis Is Killing People', 'Drugs, Inc.', 'serial killer documentary', 'самоубийство', 'ნარკოტიკები', 'Taboo: full episode']) {
    assert.ok(F.matchTitle(s).hit, `feed should leave out: ${s}`);
    assert.equal(T.matchTitle(s).hit, false, `page stays open: ${s}`);
  }
  for (const s of ['How volcanoes work', 'The Tortoise and the Hare', 'Narikala fortress', 'ციხე-სიმაგრე']) assert.equal(F.matchTitle(s).hit, false, s);
});
