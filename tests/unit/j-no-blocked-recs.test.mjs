// P2-201: nothing we recommend may lead to the block page. A rule word joined to another word (#babyyoga,
// Kinderyoga) is found, and our grid reads each video's description, as the block page does.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { parseFeed, feedDesc, unwanted, mergeLists, valuesPicks } from '../../ext/background/videos.js';

const sandbox = {};
sandbox.globalThis = sandbox;
vm.runInNewContext(readFileSync(new URL('../../ext/content/topics.js', import.meta.url), 'utf8'), sandbox);
const shared = readFileSync(new URL('../../shared/topics.json', import.meta.url), 'utf8');
const T = sandbox.KGTopics.create(JSON.parse(shared));
const F = sandbox.KGTopics.create(JSON.parse(shared), { feed: true });

test('the extension carries the same rules as shared/', () => {
  assert.equal(readFileSync(new URL('../../ext/data/topics.json', import.meta.url), 'utf8'), shared);
});

test('rule words joined to another word are found (hashtags, compounds), with the values wording', () => {
  for (const t of ['Baby stretch time #babyyoga', 'BabyYoga for newborns', 'Kinderyoga mit Mama', '#kidsyoga #fun',
    '#детскаййога', '#йогадлядетей', 'Sleep well #kidsmeditation', '#babyreiki', '#spookyhalloween', 'lovetarot reading', '#kidsmindfulness',
    // the review's misses (P2-201 review): the rule word first, Russian and Georgian joined forms, zodiac
    '#namastekids', '#mindfulnessforkids', '#детскаямедитация', '#детскиечакры', '#детскийгороскоп', '#ბავშვთაიოგა',
    '#zodiacsigns', '#myzodiac']) {
    const m = T.matchTitle(t);
    assert.equal(m.hit, true, t);
    assert.equal(m.kind, 'values', t);
  }
});

test('ordinary words that contain a rule word stay open', () => {
  for (const t of ['Karyogamy in fungi', 'The Chakraborty family picnic', 'Mutarotation of glucose', 'He was unmindful of the rain',
    'A premeditated plan', 'Christian meditation for kids', 'Йогурт для детей', 'Chinese zodiac animals and the Great Race',
    'Yogi Bear cartoon', 'ბიოგაზი', 'Biography of a chef', 'Halloween and quantum entanglement',
    // the review's must-fixes: joined names, Christian meditation joined, the Chinese zodiac (open, as in v-astrology)
    '#mithunchakraborty', '#souravchakraborty', '#vijaychakravarthy', '#biblemeditation', '#gospelmeditation',
    '#marcusaureliusmeditations', '#meditationsofmarcusaurelius', '#chinesezodiac', '#lunarnewyearzodiac',
    'Chinese zodiac story #chinesezodiac', 'Hyoga the swan knight', 'Zodiacal light over the hills', 'რადიოგადაცემა',
    '#православнаямедитация']) {
    assert.equal(T.matchTitle(t).hit, false, t);
  }
});

const XML = (desc) => `<?xml version="1.0"?><feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
<yt:channelId>UCaaaaaaaaaaaaaaaaaaaaaa</yt:channelId><title>Kids songs</title>
<entry><yt:videoId>aaaaaaaaaa1</yt:videoId><title>Gentle morning stretches</title><link rel="alternate" href="https://www.youtube.com/watch?v=aaaaaaaaaa1"/>
<author><name>Kids songs</name></author><published>2026-11-01T00:00:00+00:00</published>
<media:group><media:title>Gentle morning stretches</media:title><media:description>${desc}</media:description></media:group></entry></feed>`;

test('the feed parser keeps the start of the description and its hashtags, also the ones at the end', () => {
  const long = `${'Sing and move with us. '.repeat(40)}Subscribe for more! #babyyoga #toddlers`;
  const f = parseFeed(XML(long));
  assert.equal(f.videos.length, 1);
  const d = f.videos[0].desc;
  assert.ok(d.startsWith('Sing and move with us.'), d.slice(0, 40));
  assert.ok(d.includes('#babyyoga') && d.includes('#toddlers'), d.slice(-60));
  assert.ok(d.length <= 601, String(d.length));
  assert.equal(parseFeed(XML('')).videos[0].desc, '');
  assert.equal(feedDesc(null), '');
});

test('a video is unwanted when its title, channel or description has a rule word', () => {
  assert.equal(unwanted(F, { title: 'Gentle morning stretches', channel: 'Kids songs', desc: 'A calm kids yoga session' }), true);
  assert.equal(unwanted(F, { title: 'Gentle morning stretches', channel: 'Kids songs', desc: 'Stretch with us #babyyoga' }), true);
  assert.equal(unwanted(F, { title: 'Baby yoga', channel: 'Kids songs' }), true);
  assert.equal(unwanted(F, { title: 'Morning song', channel: 'Cosmic Kids Yoga' }), true);
  assert.equal(unwanted(F, { title: 'Morning song', channel: 'Kids songs', desc: 'Sing along every morning' }), false);
  assert.equal(unwanted(F, { title: 'Morning song', channel: 'Kids songs' }), false);
  assert.equal(unwanted(null, { title: 'Baby yoga' }), false);
});

test('the grid leaves out a feed video whose description has a rule word; the page never gets the description', () => {
  const A = 'UCaaaaaaaaaaaaaaaaaaaaaa';
  const feeds = { [A]: { videos: [
    { id: 'aaaaaaaaaa1', title: 'Gentle morning stretches', channel: 'Kids songs', published: 2, desc: 'A calm #kidsyoga session' },
    { id: 'aaaaaaaaaa2', title: 'Counting to ten', channel: 'Kids songs', published: 1, desc: 'Count with us' },
  ] } };
  const lists = mergeLists([{ id: A, lang: 'en' }], feeds, {}, F);
  assert.deepEqual(lists[0].videos.map((v) => v.id), ['aaaaaaaaaa2']);
  // Without the filter both stay (the filter does the work, not the data).
  assert.equal(mergeLists([{ id: A, lang: 'en' }], feeds, {}, null)[0].videos.length, 2);
  const src = readFileSync(new URL('../../ext/background/videos.js', import.meta.url), 'utf8');
  assert.match(src, /mixed\.map\(\(\{ src, desc, \.\.\.v \}\) => v\)/);
});

test('values picks leave out a video whose description has a rule word', () => {
  const pool = [
    { id: 'vvvvvvvvvv1', title: 'Kindness story', channel: 'Stories', lang: 'en', bands: ['3-5'], desc: 'Breathe with us #mindfulkids #kidsmindfulness' },
    { id: 'vvvvvvvvvv2', title: 'Sharing story', channel: 'Stories', lang: 'en', bands: ['3-5'], desc: 'A story about sharing' },
  ];
  const picks = valuesPicks(pool, { band: '3-5', lang: 'en', day: '2026-11-11', n: 8, topics: F });
  assert.deepEqual(picks.map((v) => v.id), ['vvvvvvvvvv2']);
});

test('the YouTube page script also reads the description line under a result', () => {
  const src = readFileSync(new URL('../../ext/content/focus-youtube.js', import.meta.url), 'utf8');
  assert.match(src, /const DESC_SEL = '[^']*\.metadata-snippet-text/);
  assert.match(src, /topicHit\(text\) \|\| \(desc && topicHit\(desc\)\)/);
});
