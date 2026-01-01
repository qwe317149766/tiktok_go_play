const crypto = require('node:crypto');

// Replicate the PRNG initialization and keyWords generation
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

// Generate 12 keyWords
const keyWords = [];
for (let i = 0; i < 12; i++) {
  const rnd = rand();
  const word = (rnd * 4294967296) >>> 0;
  keyWords.push(word);
}

console.log("JS All 12 keyWords:", keyWords.map(w => w.toString()).join(' '));


