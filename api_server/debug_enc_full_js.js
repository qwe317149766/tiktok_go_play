const crypto = require('node:crypto');
const encrypt = require('./xgnarly.js');

const queryString = "WebIdLastTime=1767083930&aid=1988&app_language=en-GB&app_name=tiktok_web&browser_language=en-IE&browser_name=Mozilla&browser_online=true&browser_platform=Win32&browser_version=5.0+%28MeeGo%3B+NokiaN9%29+AppleWebKit%2F534.13+%28KHTML%2C+like+Gecko%29+NokiaBrowser%2F8.5.0+Mobile+Safari%2F534.13&channel=tiktok_web&cookie_enabled=true&data_collection_enabled=true&device_id=7589567659773068817&device_platform=web_pc&focus_state=false&from_page=video&history_len=11&is_fullscreen=false&is_page_visible=true&itemId=7569637642548104479&msToken=w7IMDLNpgcLgWnLzHJg6UDyjSM6JlxNXTgPqVfgA82Zjs4lRWiYwCrQBtPA3I6nNNlV_87dNvgHJKbJwnEYM4y8cA72N-Qq2uNbni3pNCWvrHyrIKNdPwgn3fUQnUYZnVzkJWCLqXiBiOj_6oj3XavNqAq7r&os=unknown&priority_region=JP&referer=&region=JP&screen_height=854&screen_width=480&tz_name=Asia%2FShanghai&user_is_login=true&verifyFp=verify_mjvc6b0t_DveU8fMQ_UJGE_4bUO_Bkx4_lwVLknixAOZy&webcast_language=en-GB";
const body = "";
const userAgent = "Mozilla/5.0 (MeeGo; NokiaN9) AppleWebKit/534.13 (KHTML, like Gecko) NokiaBrowser/8.5.0 Mobile Safari/534.13";

// We need to modify xgnarly.js to export the 'enc' variable
// For now, let's manually trace through the code
// Actually, let's create a modified version that logs 'enc'

// Copy the full encrypt function logic here and add logging
const timestampMs = 1767083930000;
const ts = timestampMs;
const nowMs = ts >>> 0;

let seed = BigInt(ts);
const a = BigInt(1103515245);
const c = BigInt(12345);
const m = BigInt(0x7fffffff);

function next() {
  seed = ((seed * a + c) & m);
  return Number(seed);
}

const aa = [0xFFFFFFFF, 138, 1498001188, 211147047, 253, null, 203, 288, 9, 1196819126, 3212677781, 135, 263, 193, 58, 18, 244, 2931180889, 240, 173, 268, 2157053261, 261, 175, 14, 5, 171, 270, 156, 258, 13, 15, 3732962506, 185, 169, 2, 6, 132, 162, 200, 3, 160, 217618912, 62, 2517678443, 44, 164, 4, 96, 183, 2903579748, 3863347763, 119, 181, 10, 190, 8, 2654435769, 259, 104, 230, 128, 2633865432, 225, 1, 257, 143, 179, 16, 600974999, 185100057, 32, 188, 53, 2718276124, 177, 196, 0xFFFFFFFF, 147, 117, 17, 49, 7, 28, 12, 266, 216, 11, 0, 45, 166, 247, 1451689750];

const kt = [aa[44], aa[74], aa[10], aa[62], aa[42], aa[17], aa[2], aa[21], aa[3], aa[70], aa[50], aa[32], aa[0] & nowMs, next() % aa[77], next() % aa[77], next() % aa[77]];
let St = aa[88];

const u32 = x => (x >>> 0);
const rotl = (x, n) => u32((x << n) | (x >>> (32 - n)));

function quarter(st, a, b, c, d) {
  st[a] = u32(st[a] + st[b]);
  st[d] = rotl(st[d] ^ st[a], 16);
  st[c] = u32(st[c] + st[d]);
  st[b] = rotl(st[b] ^ st[c], 12);
  st[a] = u32(st[a] + st[b]);
  st[d] = rotl(st[d] ^ st[a], 8);
  st[c] = u32(st[c] + st[d]);
  st[b] = rotl(st[b] ^ st[c], 7);
}

function chachaBlock(state, rounds) {
  const w = state.slice();
  let r = 0;
  while (r < rounds) {
    quarter(w, 0, 4, 8, 12);
    quarter(w, 1, 5, 9, 13);
    quarter(w, 2, 6, 10, 14);
    quarter(w, 3, 7, 11, 15);
    r++;
    if (r >= rounds) break;
    quarter(w, 0, 5, 10, 15);
    quarter(w, 1, 6, 11, 12);
    quarter(w, 2, 7, 12, 13);
    quarter(w, 3, 4, 13, 14);
    r++;
  }
  for (let i = 0; i < 16; i++) w[i] = u32(w[i] + state[i]);
  return w;
}

const bumpCounter = st => { st[12] = u32(st[12] + 1); };

function rand() {
  const e = chachaBlock(kt, 8);
  const t = e[St];
  const r = (e[St + 8] & 0xFFFFFFF0) >>> 11;
  if (St === 7) { bumpCounter(kt); St = 0; } else { ++St; }
  return (t + 4294967296 * r) / (2 ** 53);
}

function numToBytes(val) {
  if (val < 255 * 255) {
    return [(val >> 8) & 0xFF, val & 0xFF];
  }
  return [(val >> 24) & 0xFF, (val >> 16) & 0xFF, (val >> 8) & 0xFF, val & 0xFF];
}

function beIntFromStr(str) {
  const buf = Buffer.from(str, 'utf8').subarray(0, 4);
  let acc = 0;
  for (const b of buf) acc = (acc << 8) | b;
  return acc >>> 0;
}

const obj = new Map();
obj.set(1, 1);
obj.set(2, 0);
obj.set(3, crypto.createHash('md5').update(queryString).digest('hex'));
obj.set(4, crypto.createHash('md5').update(body).digest('hex'));
obj.set(5, crypto.createHash('md5').update(userAgent).digest('hex'));
obj.set(6, Math.floor(timestampMs / 1000));
obj.set(7, 1508145731);
obj.set(8, (timestampMs * 1000) % 2147483648);
obj.set(9, "5.1.1");
obj.set(10, '1.0.0.314');
obj.set(11, 1);
let v12 = 0;
for (let i = 1; i <= 11; i++) {
  const v = obj.get(i);
  const toXor = typeof v === 'number' ? v : beIntFromStr(v);
  v12 ^= toXor;
}
obj.set(12, v12 >>> 0);

let v0 = 0;
for (let i = 1; i <= obj.size; i++) {
  const v = obj.get(i);
  if (typeof v === 'number') v0 ^= v;
}
obj.set(0, v0 >>> 0);

const payload = [];
payload.push(obj.size);
for (const [k, v] of obj) {
  payload.push(k);
  const valBytes = typeof v === 'number' ? numToBytes(v) : Array.from(Buffer.from(v, 'utf8'));
  payload.push(...numToBytes(valBytes.length));
  payload.push(...valBytes);
}
const baseStr = String.fromCharCode(...payload);

const keyWords = [];
for (let i = 0; i < 12; i++) {
  const rnd = rand();
  const word = (rnd * 4294967296) >>> 0;
  keyWords.push(word);
}

const keyBytes = [];
for (const w of keyWords) {
  keyBytes.push(w & 0xFF, (w >> 8) & 0xFF, (w >> 16) & 0xFF, (w >> 24) & 0xFF);
}

const insertPos = Math.floor(rand() * (baseStr.length + 1));
const rounds = 7 + Math.floor(rand() * 8);

const Ot = [aa[9], aa[69], aa[51], aa[92]];

function encryptChaCha(keyWords, rounds, bytes) {
  const nFull = Math.floor(bytes.length / 4);
  const leftover = bytes.length % 4;
  const words = new Array(nFull + (leftover ? 1 : 0));
  for (let i = 0; i < nFull; i++) {
    const j = 4 * i;
    words[i] = (bytes[j] | (bytes[j + 1] << 8) | (bytes[j + 2] << 16) | (bytes[j + 3] << 24)) >>> 0;
  }
  if (leftover) {
    let w = 0;
    const base = 4 * nFull;
    for (let c = 0; c < leftover; c++) w |= bytes[base + c] << (8 * c);
    words[nFull] = w >>> 0;
  }

  let o = 0;
  const state = keyWords.slice();
  while (o + 16 < words.length) {
    const stream = chachaBlock(state, rounds);
    bumpCounter(state);
    for (let k = 0; k < 16; k++) words[o + k] ^= stream[k];
    o += 16;
  }
  const remain = words.length - o;
  const stream = chachaBlock(state, rounds);
  for (let k = 0; k < remain; k++) words[o + k] ^= stream[k];

  for (let i = 0; i < nFull; i++) {
    const w = words[i];
    const j = 4 * i;
    bytes[j] = w & 0xFF;
    bytes[j + 1] = (w >> 8) & 0xFF;
    bytes[j + 2] = (w >> 16) & 0xFF;
    bytes[j + 3] = (w >> 24) & 0xFF;
  }
  if (leftover) {
    const w = words[nFull];
    const base = 4 * nFull;
    for (let c = 0; c < leftover; c++) bytes[base + c] = (w >> (8 * c)) & 0xFF;
  }
}

function Ab22(key12Words, rounds, str) {
  const state = Ot.concat(key12Words);
  const data = Array.from(str, ch => ch.charCodeAt(0));
  encryptChaCha(state, rounds, data);
  return String.fromCharCode(...data);
}

const enc = Ab22(keyWords, rounds, baseStr);

console.log("JS enc (all bytes as hex):", Array.from(enc, ch => ch.charCodeAt(0)).map(b => b.toString(16).padStart(2, '0')).join(' '));
console.log("JS enc (all bytes as decimal):", Array.from(enc, ch => ch.charCodeAt(0)).join(' '));


