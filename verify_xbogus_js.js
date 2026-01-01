const encrypt = require('./api_server/xbogus.js');

const params = "device_platform=android&aid=1233";
const postData = "testbody";
const userAgent = "testua";
const timestamp = 1704067200; // 2024-01-01 00:00:00 UTC

const result = encrypt(params, postData, userAgent, timestamp);
console.log(result);
