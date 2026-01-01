const crypto = require('node:crypto');

const queryString = "WebIdLastTime=1767083930&aid=1988&app_language=en-GB&app_name=tiktok_web&browser_language=en-IE&browser_name=Mozilla&browser_online=true&browser_platform=Win32&browser_version=5.0+%28MeeGo%3B+NokiaN9%29+AppleWebKit%2F534.13+%28KHTML%2C+like+Gecko%29+NokiaBrowser%2F8.5.0+Mobile+Safari%2F534.13&channel=tiktok_web&cookie_enabled=true&data_collection_enabled=true&device_id=7589567659773068817&device_platform=web_pc&focus_state=false&from_page=video&history_len=11&is_fullscreen=false&is_page_visible=true&itemId=7569637642548104479&msToken=w7IMDLNpgcLgWnLzHJg6UDyjSM6JlxNXTgPqVfgA82Zjs4lRWiYwCrQBtPA3I6nNNlV_87dNvgHJKbJwnEYM4y8cA72N-Qq2uNbni3pNCWvrHyrIKNdPwgn3fUQnUYZnVzkJWCLqXiBiOj_6oj3XavNqAq7r&os=unknown&priority_region=JP&referer=&region=JP&screen_height=854&screen_width=480&tz_name=Asia%2FShanghai&user_is_login=true&verifyFp=verify_mjvc6b0t_DveU8fMQ_UJGE_4bUO_Bkx4_lwVLknixAOZy&webcast_language=en-GB";
const body = "";
const userAgent = "Mozilla/5.0 (MeeGo; NokiaN9) AppleWebKit/534.13 (KHTML, like Gecko) NokiaBrowser/8.5.0 Mobile Safari/534.13";
const envcode = 0;
const version = "5.1.1";
const timestampMs = 1767083930000;

// Replicate the logic from xgnarly.js
const aa = [0xFFFFFFFF, 138, 1498001188, 211147047, 253, null, 203, 288, 9, 1196819126, 3212677781, 135, 263, 193, 58, 18, 244, 2931180889, 240, 173, 268, 2157053261, 261, 175, 14, 5, 171, 270, 156, 258, 13, 15, 3732962506, 185, 169, 2, 6, 132, 162, 200, 3, 160, 217618912, 62, 2517678443, 44, 164, 4, 96, 183, 2903579748, 3863347763, 119, 181, 10, 190, 8, 2654435769, 259, 104, 230, 128, 2633865432, 225, 1, 257, 143, 179, 16, 600974999, 185100057, 32, 188, 53, 2718276124, 177, 196, 0xFFFFFFFF, 147, 117, 17, 49, 7, 28, 12, 266, 216, 11, 0, 45, 166, 247, 1451689750];
const Ot = [aa[9], aa[69], aa[51], aa[92]];
const MASK32 = 0xFFFFFFFF;

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

// Build obj
const obj = new Map();
obj.set(1, 1);
obj.set(2, envcode);
obj.set(3, crypto.createHash('md5').update(queryString).digest('hex'));
obj.set(4, crypto.createHash('md5').update(body).digest('hex'));
obj.set(5, crypto.createHash('md5').update(userAgent).digest('hex'));
obj.set(6, Math.floor(timestampMs / 1000));
obj.set(7, 1508145731);
obj.set(8, (timestampMs * 1000) % 2147483648);
obj.set(9, version);

if (version === '5.1.1') {
  obj.set(10, '1.0.0.314');
  obj.set(11, 1);
  let v12 = 0;
  for (let i = 1; i <= 11; i++) {
    const v = obj.get(i);
    const toXor = typeof v === 'number' ? v : beIntFromStr(v);
    v12 ^= toXor;
  }
  obj.set(12, v12 >>> 0);
}

// Compute v0
let v0 = 0;
for (let i = 1; i <= obj.size; i++) {
  const v = obj.get(i);
  if (typeof v === 'number') v0 ^= v;
}
obj.set(0, v0 >>> 0);

// Serialize payload
const payload = [];
payload.push(obj.size);
for (const [k, v] of obj) {
  payload.push(k);
  const valBytes = typeof v === 'number' ? numToBytes(v) : Array.from(Buffer.from(v, 'utf8'));
  payload.push(...numToBytes(valBytes.length));
  payload.push(...valBytes);
}
const baseStr = String.fromCharCode(...payload);

console.log("baseStr length:", baseStr.length);
console.log("baseStr (first 50 bytes as hex):", Array.from(baseStr.slice(0, 50), ch => ch.charCodeAt(0)).map(b => b.toString(16).padStart(2, '0')).join(' '));


