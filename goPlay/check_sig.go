package main

import (
	"fmt"
	"tt_code/headers"
)

func main() {
	qs := "req_id=1d123522-3e4e-4e7d-9e4f-3e2b190e4e96&device_platform=android&os=android&ssmix=a&_rticket=1767424033997&cdid=42857e2a-b162-49ab-a3f0-3f709f3873f4&channel=googleplay&aid=1233&app_name=musical_ly&version_code=400603&version_name=40.6.3&manifest_version_code=2024006030&update_version_code=2024006030&ab_version=40.6.3&resolution=1080*2029&dpi=440&device_type=MI%208&device_brand=Xiaomi&language=zh-Hant&os_api=29&os_version=10&ac=wifi&is_pad=0&app_type=normal&sys_region=TW&last_install_time=1767424023&timezone_name=Asia%2FYerevan&app_language=zh-Hant&timezone_offset=14400&host_abi=arm64-v8a&locale=zh-Hant-TW&ac2=unknown&uoo=1&op_region=TW&build_number=40.6.3&region=TW&ts=1767424033&openudid=b0049f7a25806c51&okhttp_version=4.2.228.18-tiktok&use_store_region_cookie=1"
	stub := "799968CE27B184778C807AF28435A589"
	ts := int64(1767424034)

	h := headers.MakeHeaders(
		"",
		ts,
		1, 0, 0, ts,
		"", "MI 8", "", 0, "", "", "",
		qs,
		stub, // MakeHeaders takes hex postData if it's POST, wait.
		"40.6.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	fmt.Printf("Expected:  8404c0cb00004b537cec54d302731115245883170da1bb086858\n")
	fmt.Printf("Generated: %s\n", h.XGorgon)

	if h.XGorgon == "8404c0cb00004b537cec54d302731115245883170da1bb086858" {
		fmt.Println("✅ Signature MATCHES!")
	} else {
		fmt.Println("❌ Signature MISMATCH!")
	}
}
