const crypto = require('node:crypto');
const encrypt = require('./xgnarly.js');

// We need to modify xgnarly.js to export intermediate values
// For now, let's create a test version that logs enc
const queryString = "WebIdLastTime=1767083930&aid=1988&app_language=en-GB&app_name=tiktok_web&browser_language=en-IE&browser_name=Mozilla&browser_online=true&browser_platform=Win32&browser_version=5.0+%28MeeGo%3B+NokiaN9%29+AppleWebKit%2F534.13+%28KHTML%2C+like+Gecko%29+NokiaBrowser%2F8.5.0+Mobile+Safari%2F534.13&channel=tiktok_web&cookie_enabled=true&data_collection_enabled=true&device_id=7589567659773068817&device_platform=web_pc&focus_state=false&from_page=video&history_len=11&is_fullscreen=false&is_page_visible=true&itemId=7569637642548104479&msToken=w7IMDLNpgcLgWnLzHJg6UDyjSM6JlxNXTgPqVfgA82Zjs4lRWiYwCrQBtPA3I6nNNlV_87dNvgHJKbJwnEYM4y8cA72N-Qq2uNbni3pNCWvrHyrIKNdPwgn3fUQnUYZnVzkJWCLqXiBiOj_6oj3XavNqAq7r&os=unknown&priority_region=JP&referer=&region=JP&screen_height=854&screen_width=480&tz_name=Asia%2FShanghai&user_is_login=true&verifyFp=verify_mjvc6b0t_DveU8fMQ_UJGE_4bUO_Bkx4_lwVLknixAOZy&webcast_language=en-GB";
const body = "";
const userAgent = "Mozilla/5.0 (MeeGo; NokiaN9) AppleWebKit/534.13 (KHTML, like Gecko) NokiaBrowser/8.5.0 Mobile Safari/534.13";

// We'll need to manually trace through the code
// Actually, let's modify xgnarly.js temporarily to export debug info
// For now, just call encrypt and see the result
const result = encrypt(queryString, body, userAgent, 0, "5.1.1", 1767083930000);
console.log("JS result:", result.substring(0, 50));


