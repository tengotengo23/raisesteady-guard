// page-check classifier, both ways (adult pages blocked; health, sex education, art, news pass).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const src = readFileSync(new URL('../../ext/content/classify.js', import.meta.url), 'utf8');
const sandbox = {};
sandbox.globalThis = sandbox;
vm.runInNewContext(src, sandbox);
const { classify } = sandbox.KGClassify;

const page = (o) => ({ title: '', description: '', keywords: '', headings: [], text: '', url: 'example.org/', metas: [], ...o });

const BLOCK = {
  'RTA label in a meta tag': page({ title: 'Members area', metas: [{ name: 'RATING', content: 'RTA-5042-1996-1400-1577-RTA' }], text: 'Welcome back.' }),
  'rating meta adult': page({ title: 'Gallery', metas: [{ name: 'rating', content: 'adult' }] }),
  'English tube site': page({
    title: 'Free Porn Videos & XXX Sex Movies - HD Tube',
    description: 'Watch free hardcore porn videos, MILF and amateur sex tube clips updated daily.',
    headings: ['Most viewed', 'Hot MILF videos', 'Live sex cams'],
    text: 'Free porn videos. xxx amateur blowjob compilation 12:04. MILF gets ... Cam girls online now. Big tits HD. 18+ only. Sex videos added every hour. Gangbang. Hentai.',
    url: 'freeporntube.example/',
  }),
  'English cam site': page({
    title: 'Live Sex Cams with Hot Cam Girls',
    description: 'Free live sex chat with nude girls. NSFW.',
    text: 'Thousands of cam girls are online. Horny models, nude girls and hot sluts waiting in private sex chat. You must be 18 to enter. NSFW content.',
    url: 'camsite.example/',
  }),
  'Georgian adult page': page({
    title: 'ქართული პორნო ვიდეოები - სექს ვიდეო ონლაინ',
    description: 'უფასო პორნო, ეროტიკა და სექს ჩატი. მხოლოდ 18+.',
    headings: ['ახალი პორნო ვიდეოები', 'ქართველი ბოზები'],
    text: 'უყურე უფასო პორნო ვიდეოებს. სექსი ონლაინ. ეროტიული ფოტოები, შიშველი გოგოები. ბოზები შენს ქალაქში. XXX ვიდეოები ყოველდღე.',
    url: 'example.ge/video',
  }),
  'Russian adult page': page({
    title: 'Порно видео бесплатно — секс видео онлайн',
    description: 'Смотреть порно онлайн, минет, голые девушки, эротика, секс чат.',
    headings: ['Новое порно', 'Голые девушки', 'Шлюхи рядом'],
    text: 'Бесплатное порно видео в хорошем качестве. Минет, домашнее порно, сиськи, трахает подругу. Секс знакомства и вебкам модели онлайн. Только для взрослых 18+.',
    url: 'example.ru/',
  }),
};

const PASS = {
  'sex education (en)': page({
    title: 'Puberty and sex education for teenagers',
    description: 'Clear answers about puberty, consent, contraception and sexual health from doctors.',
    headings: ['What happens during puberty', 'Sex and consent', 'Masturbation is normal', 'Sexually transmitted infections'],
    text: 'Puberty is when your body changes. Sex is something to talk about with a parent or a doctor. Masturbation is normal and healthy. Using condoms and other contraception prevents pregnancy and STIs such as HIV. Consent means saying yes freely. Your school nurse or clinic can help. Naked bodies all look different, and that is fine. Some people have an orgasm, some do not; both are normal. Oral sex and anal sex can pass on infections too.',
    url: 'health.example.org/puberty',
  }),
  'breast cancer screening': page({
    title: 'Breast cancer screening: what to expect',
    description: 'Mammogram screening for breast cancer at your local hospital.',
    headings: ['Who is invited', 'The mammogram', 'Results'],
    text: 'Breast screening uses a mammogram to look for cancer early. You will be asked to undress from the waist up. A female practitioner takes images of each breast. Results arrive within two weeks. Talk to your doctor if you notice changes in your breasts or nipples.',
    url: 'nhs.example/breast-screening',
  }),
  'anatomy lesson': page({
    title: 'Human reproductive system - biology lesson',
    headings: ['The male reproductive system', 'The female reproductive system', 'Fertilization'],
    text: 'The penis and testes produce and deliver sperm. The vagina, uterus and ovaries form the female reproductive system. During sexual intercourse sperm travels to the egg. This anatomy lesson for school students covers puberty and hormones.',
    url: 'school.example/biology',
  }),
  'art history: the nude': page({
    title: 'The nude in Renaissance painting',
    description: 'Art history: how painters from Titian to Botticelli painted the naked body.',
    headings: ['Venus of Urbino', 'The Birth of Venus', 'Erotic or sacred?'],
    text: 'The female nude was a central subject of Renaissance painting. Titian’s Venus of Urbino has been read as erotic, yet the museum catalogue stresses its marriage symbolism. Michelangelo’s David is a nude sculpture. Art historians discuss nudes in the gallery context.',
    url: 'museum.example/essays/nude',
  }),
  'news about crime (en)': page({
    title: 'Man jailed for sharing child pornography',
    description: 'A court sentenced the man after a police investigation.',
    headings: ['Police investigation', 'Sentencing'],
    text: 'A man has been jailed for eight years for possessing and sharing child pornography, the court heard. Police arrested him after an investigation. The judge said pornography of this kind causes lasting harm. Prosecutors said he had used encrypted apps. Charities urged parents to talk to children about online safety. Revenge porn laws were also discussed.',
    url: 'news.example/crime/123',
  }),
  'news about porn law (en)': page({
    title: 'New law requires age checks on porn sites',
    description: 'Parliament passes legislation on age verification for adult websites.',
    text: 'Porn sites must check the age of visitors under the new law. OnlyFans and other platforms said they would comply. Campaigners said children see pornography too early. The regulator will investigate sites that fail, and courts can fine them. Parents welcomed the legislation.',
    url: 'news.example/politics/age-checks',
  }),
  'Georgian health page': page({
    title: 'სქესობრივი მომწიფება: რა ხდება სხეულში',
    description: 'ექიმის რჩევები მოზარდებსა და მშობლებს სქესობრივი ჯანმრთელობის შესახებ.',
    headings: ['მომწიფება', 'კონტრაცეფცია', 'სქესობრივი გზით გადამდები ინფექციები'],
    text: 'სქესობრივი მომწიფება ბუნებრივი პროცესია. ექიმი და მშობლები დაგეხმარებიან. მასტურბაცია ნორმალურია. კონტრაცეფცია იცავს არასასურველი ორსულობისგან. სკოლაში ბიოლოგიის გაკვეთილზე ვსწავლობთ ანატომიას.',
    url: 'health.example.ge/',
  }),
  'Russian news about crime': page({
    title: 'Задержан администратор сайта с детской порнографией',
    description: 'Полиция провела расследование, дело передано в суд.',
    text: 'Полиция задержала администратора сайта, распространявшего детскую порнографию. Возбуждено уголовное дело, расследование продолжается. Суд арестовал обвиняемого. Эксперты советуют родителям говорить с детьми о безопасности в интернете.',
    url: 'news.example.ru/',
  }),
  'Russian anatomy page': page({
    title: 'Половое созревание у подростков',
    description: 'Врач рассказывает о половом созревании и здоровье.',
    text: 'Половое созревание — нормальный этап. Анатомия половых органов, гигиена, интимное здоровье. Секс и контрацепция: врач объясняет, как защитить здоровье. Обратитесь к врачу в клинику.',
    url: 'clinic.example.ru/',
  }),
  'ordinary page with "Sussex" and a cooking "cream pie"': page({
    title: 'Banana cream pie recipe from Sussex',
    text: 'A classic banana cream pie. Hardcore bakers will love it. Serves eight.',
    url: 'sussexkitchen.example/cream-pie',
  }),
};

for (const [name, p] of Object.entries(BLOCK)) {
  test(`blocks: ${name}`, () => {
    const r = classify(p);
    assert.equal(r.block, true, JSON.stringify(r));
  });
}
for (const [name, p] of Object.entries(PASS)) {
  test(`passes: ${name}`, () => {
    const r = classify(p);
    assert.equal(r.block, false, JSON.stringify(r));
  });
}
test('one strong term alone never blocks', () => {
  assert.equal(classify(page({ title: 'Porn addiction: how to get help', text: 'Porn can affect teenagers. Talk to a counsellor.' })).block, false);
});
