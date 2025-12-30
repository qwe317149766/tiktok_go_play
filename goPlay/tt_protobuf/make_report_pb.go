package tt_protobuf

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

func randomUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff)
}

const (
	reportTwoFeature    = "feature:reqGlEsVersion=0x30002 feature:android.hardware.audio.low_latency feature:android.hardware.audio.output feature:android.hardware.audio.pro feature:android.hardware.bluetooth feature:android.hardware.bluetooth_le feature:android.hardware.camera feature:android.hardware.camera.any feature:android.hardware.camera.autofocus feature:android.hardware.camera.capability.manual_post_processing feature:android.hardware.camera.capability.manual_sensor feature:android.hardware.camera.capability.raw feature:android.hardware.camera.concurrent feature:android.hardware.camera.flash feature:android.hardware.camera.front feature:android.hardware.camera.level.full feature:android.hardware.context_hub feature:android.hardware.device_unique_attestation feature:android.hardware.faketouch feature:android.hardware.fingerprint feature:android.hardware.hardware_keystore=300 feature:android.hardware.keystore.app_attest_key feature:android.hardware.location feature:android.hardware.location.gps feature:android.hardware.location.network feature:android.hardware.microphone feature:android.hardware.nfc feature:android.hardware.nfc.any feature:android.hardware.nfc.ese feature:android.hardware.nfc.hce feature:android.hardware.nfc.hcef feature:android.hardware.nfc.uicc feature:android.hardware.opengles.aep feature:android.hardware.ram.normal feature:android.hardware.screen.landscape feature:android.hardware.screen.portrait feature:android.hardware.se.omapi.ese feature:android.hardware.se.omapi.uicc feature:android.hardware.security.model.compatible feature:android.hardware.sensor.accelerometer feature:android.hardware.sensor.barometer feature:android.hardware.sensor.compass feature:android.hardware.sensor.dynamic.head_tracker feature:android.hardware.sensor.gyroscope feature:android.hardware.sensor.hifi_sensors feature:android.hardware.sensor.light feature:android.hardware.sensor.proximity feature:android.hardware.sensor.stepcounter feature:android.hardware.sensor.stepdetector feature:android.hardware.strongbox_keystore=300 feature:android.hardware.telephony feature:android.hardware.telephony.calling feature:android.hardware.telephony.carrierlock feature:android.hardware.telephony.data feature:android.hardware.telephony.euicc feature:android.hardware.telephony.gsm feature:android.hardware.telephony.ims feature:android.hardware.telephony.ims.singlereg feature:android.hardware.telephony.messaging feature:android.hardware.telephony.radio.access feature:android.hardware.telephony.subscription feature:android.hardware.touchscreen feature:android.hardware.touchscreen.multitouch feature:android.hardware.touchscreen.multitouch.distinct feature:android.hardware.touchscreen.multitouch.jazzhand feature:android.hardware.usb.accessory feature:android.hardware.usb.host feature:android.hardware.vulkan.compute feature:android.hardware.vulkan.level=1 feature:android.hardware.vulkan.version=4206592 feature:android.hardware.wifi feature:android.hardware.wifi.aware feature:android.hardware.wifi.direct feature:android.hardware.wifi.passpoint feature:android.hardware.wifi.rtt feature:android.software.activities_on_secondary_displays feature:android.software.app_enumeration feature:android.software.app_widgets feature:android.software.autofill feature:android.software.backup feature:android.software.cant_save_state feature:android.software.companion_device_setup feature:android.software.controls feature:android.software.credentials feature:android.software.cts feature:android.software.device_admin feature:android.software.device_id_attestation feature:android.software.device_lock feature:android.software.erofs feature:android.software.file_based_encryption feature:android.software.game_service feature:android.software.home_screen feature:android.software.incremental_delivery=2 feature:android.software.input_methods feature:android.software.ipsec_tunnel_migration feature:android.software.ipsec_tunnels feature:android.software.live_wallpaper feature:android.software.managed_users feature:android.software.midi feature:android.software.opengles.deqp.level=132645633 feature:android.software.picture_in_picture feature:android.software.print feature:android.software.secure_lock_screen feature:android.software.securely_removes_users feature:android.software.telecom feature:android.software.verified_boot feature:android.software.virtualization_framework feature:android.software.voice_recognizers feature:android.software.vulkan.deqp.level=132645633 feature:android.software.wallet_location_based_suggestions feature:android.software.webview feature:android.software.window_magnification feature:com.android.systemui.SUPPORTS_DRAG_ASSISTANT_TO_SPLIT feature:com.google.android.apps.dialer.SUPPORTED feature:com.google.android.feature.ADAPTIVE_CHARGING feature:com.google.android.feature.AER_OPTIMIZED feature:com.google.android.feature.CONTEXTUAL_SEARCH feature:com.google.android.feature.D2D_CABLE_MIGRATION_FEATURE feature:com.google.android.feature.DREAMLINER feature:com.google.android.feature.EXCHANGE_6_2 feature:com.google.android.feature.GMS_GAME_SERVICE feature:com.google.android.feature.GOOGLE_BUILD feature:com.google.android.feature.GOOGLE_EXPERIENCE feature:com.google.android.feature.GOOGLE_FI_BUNDLED feature:com.google.android.feature.NEXT_GENERATION_ASSISTANT feature:com.google.android.feature.PIXEL_2017_EXPERIENCE feature:com.google.android.feature.PIXEL_2018_EXPERIENCE feature:com.google.android.feature.PIXEL_2019_EXPERIENCE feature:com.google.android.feature.PIXEL_2019_MIDYEAR_EXPERIENCE feature:com.google.android.feature.PIXEL_2020_EXPERIENCE feature:com.google.android.feature.PIXEL_2020_MIDYEAR_EXPERIENCE feature:com.google.android.feature.PIXEL_2021_EXPERIENCE feature:com.google.android.feature.PIXEL_2021_MIDYEAR_EXPERIENCE feature:com.google.android.feature.PIXEL_EXPERIENCE feature:com.google.android.feature.QUICK_TAP feature:com.google.android.feature.TURBO_PRELOAD feature:com.google.android.feature.WELLBEING feature:com.nxp.mifare feature:com.verizon.hardware.telephony.ehrpd feature:com.verizon.hardware.telephony.lte feature:vendor.android.hardware.camera.preview-dis.back feature:vendor.android.hardware.camera.preview-dis.front"
	reportTwoLibs       = "library:android.ext.shared library:android.hidl.base-V1.0-java library:android.hidl.manager-V1.0-java library:android.net.ipsec.ike library:android.telephony.satellite library:android.test.base library:android.test.mock library:android.test.runner library:androidx.camera.extensions.impl library:androidx.window.extensions library:androidx.window.sidecar library:com.android.cts.ctsshim.shared_library library:com.android.future.usb.accessory library:com.android.hotwordenrollment.common.util library:com.android.location.provider library:com.android.media.remotedisplay library:com.android.mediadrm.signer library:com.android.nfc_extras library:com.android.omadm.radioconfig library:com.google.android.apps.aicore library:com.google.android.camera.experimental2021 library:com.google.android.camera.experimental2022_system library:com.google.android.camera.extensions library:com.google.android.camerax.extensions library:com.google.android.dialer.support library:com.google.android.gms library:com.google.android.hardwareinfo library:com.google.pixel.camera.connectivity library:com.google.pixel.camera.connectivity.impl library:com.google.pixel.camera.services.cameraidremapper library:com.google.pixel.camera.services.cameraidremapper.impl library:com.google.pixel.camera.services.lyricconfigprovider library:com.google.pixel.camera.services.lyricconfigprovider.impl library:com.vzw.apnlib library:google-ril library:javax.obex library:libOpenCL-pixel.so library:libOpenCL.so library:lib_aion_buffer.so library:libedgetpu_client.google.so library:libedgetpu_util.so library:libmetrics_logger.so library:oemrilhook library:org.apache.http.legacy"
	deviceSecurityJSON  = `{"lc":"1","vbds":"locked","vbs":"green"}`
	stHWJSON            = `{"st":"/gSdNeRQ2fr0smeIhk97X9W4VydtUnGwZ+8OAmis/28=","hw":""}`
	reportSevenInternet = `{"iip":"192.168.15.54","gip":"192.168.15.26","ghw":"","type":"wlan0"}`
	reportTenUnknown4   = `{"f_fd":0,"add":0,"thd":0,"cba":0,"p_dso":1,"bnd":0}`
)

const reportTenUnknown1BaseTemplate = `{"rd":"%s","ksir_code":"%s","dt_no":3,"rt_no":2,"dt_fs":4076150800,"rt_fs":61267,"dt_dn":"/dev/block/dm-52","rt_dn":"/dev/block/dm-6","vx_f":0,"mt_ap":0,"lbdc":"0","ibdc":"-1","ebdc":"0","vpc":"0","vps":"","amn":"android.app.IActivityManager","dmt":"%dl","dat":"%dl","slr":"0x6e6e1352fc","slr_m":"/data/app/~~z9cmENdIZ6DjwE3OJY0FFA==/com.zhiliaoapp.musically-bwSP0qv0kvxCEuFT8Gv9qg==/lib/arm64/libmetasec_ov.so","hlr":"0x7327169860","hlr_m":"[vdso]","rlr":"0x6e6e0b4964","rlr_m":"/data/app/~~z9cmENdIZ6DjwE3OJY0FFA==/com.zhiliaoapp.musically-bwSP0qv0kvxCEuFT8Gv9qg==/lib/arm64/libmetasec_ov.so","crc":1,"spc":"2","mgk":"0","npn":"0","_m_ar":"0","frd_msg":"[],[],[]","self_maps":"","ahk_t":"!error!","sg_s":1,"ak_cl":";3355cd3_20251106","ak_fl":";AndroidManifest.xml;;DebugProbesKt.bin;;META-INF;;app-update.properties;;assets;;billing.properties;;classes.dex;;classes10.dex;;classes11.dex;;classes12.dex;;classes13.dex;;classes14.dex;;classes15.dex;;classes16.dex;;classes17.dex;;classes18.dex;;classes19.dex;;classes2.dex;;classes20.dex;;classes21.dex;;classes22.dex;;classes23.dex;;classes24.dex;;classes25.dex;;classes26.dex;;classes27.dex;;classes28.dex;;classes29.dex;;classes3.dex;;classes30.dex;;classes31.dex;;classes32.dex;;classes33.dex;;classes34.dex;;classes35.dex;;classes36.dex;;classes37.dex;;classes38.dex;;classes39.dex;;classes4.dex;;classes40.dex;;classes41.dex;;classes42.dex;;classes43.dex;;classes44.dex;;classes45.dex;;classes46.dex;;classes5.dex;;classes6.dex;;classes7.dex;;classes8.dex;;classes9.dex;;com;;core-common.properties;;engage-core.properties;;feature-delivery.properties;;firebase-analytics.properties;;firebase-annotations.properties;;firebase-common.properties;;firebase-core.properties;;firebase-datatransport.properties;;firebase-encoders-json.properties;;firebase-encoders-proto.properties;;firebase-encoders.properties;;firebase-iid-interop.properties;;firebase-installations-interop.properties;;firebase-installations.properties;;firebase-measurement-connector.properties;;firebase-messaging.properties;;google-http-client.properties;;google;;googleid.properties;;integrity.properties;;jacoco-agent.properties;;lib;;messages.properties;;oberon_affected_files.txt;;org;;play-services-ads-base.properties;;play-services-ads-identifier.properties;;play-services-auth-api-phone.properties;;play-services-auth-base.properties;;play-services-auth-blockstore.properties;;play-services-auth.properties;;play-services-base.properties;;play-services-basement.properties;;play-services-cloud-messaging.properties;;play-services-fido.properties;;play-services-gass.properties;;play-services-identity.properties;;play-services-location.properties;;play-services-maps.properties;;play-services-measurement-api.properties;;play-services-measurement-base.properties;;play-services-measurement-impl.properties;;play-services-measurement-sdk-api.properties;;play-services-measurement-sdk.properties;;play-services-measurement.properties;;play-services-places-placereport.properties;;play-services-stats.properties;;play-services-tapandpay.properties;;play-services-tasks.properties;;play-services-wallet.properties;;report.css;;res;;resources.arsc;;scoped-kotlin-compiler.log;;transport-api.properties;;transport-backend-cct.properties;;transport-runtime.properties;","path":"package:/data/app/~~z9cmENdIZ6DjwE3OJY0FFA==/com.zhiliaoapp.musically-bwSP0qv0kvxCEuFT8Gv9qg==/base.apk","ak_ph":"/data/app/~~z9cmENdIZ6DjwE3OJY0FFA==/com.zhiliaoapp.musically-bwSP0qv0kvxCEuFT8Gv9qg==/base.apk","v2sign":"194326e82c84a639a52e5c023116f12a","ak_sh":"b4bbbbfc566676e47550faef93c30190e2e8ad81","sign":"194326e82c84a639a52e5c023116f12a","sg_all":"194326e82c84a639a52e5c023116f12a","sha1":"d79f7cb8509a5e7e71c4f2afcfb75ea8c87177ca","lc_sg":"0","snr":"","sg":1213,"sp":0,"atify":"0x00000073271b68f0","notify":3596551104,"cba":"0x00000073271d04d8","dp":"1020902980,0,-1","dump":0,"dump2":-1,"ts1":0,"ncl":"["wlan0"]","ntn":"0","lch":"bc897e644ef90e010bc2f7fb182866565eddd5aa","lcp":"/system/lib64/libc.so","_m_lp":"0","_m_zg":"0","_m_mo":"0","setting_version":0,"garbled":false}`

func buildReportTenUnknown1(stime int64, hasIntegrity bool) string {
	rd := `{"pass_agegate_13":1}`
	if hasIntegrity {
		rd = `{"logged":1,"pass_agegate_13":1}`
	}
	rdEscaped := strings.ReplaceAll(rd, `"`, `\"`)
	return fmt.Sprintf(reportTenUnknown1BaseTemplate, rdEscaped, randomHexString(4), stime, stime)
}

// EncodeReportTwoSixteen 编码ReportTwoSixteen消息
func EncodeReportTwoSixteen(r *ReportTwoSixteen) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteFixed64(6, r.Unknown6)
	return e.Bytes()
}

// EncodeReportTwo 编码ReportTwo消息
func EncodeReportTwo(r *ReportTwo) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(r.UnknownUtime))
	e.WriteInt64(2, int64(r.Unknown1))
	e.WriteInt64(3, int64(r.NCPU))
	e.WriteString(5, r.PinlvCPUMax)
	e.WriteString(6, r.PinlvCPUMin)
	e.WriteInt64(7, int64(r.S1))
	e.WriteInt64(8, int64(r.S2))
	e.WriteString(9, r.Chuliqi)
	e.WriteString(10, r.Netset1)
	e.WriteString(11, r.DeviceType)
	e.WriteString(12, r.TwoName1)
	e.WriteString(13, r.TwoName2)
	e.WriteString(14, r.TwoName3)
	e.WriteString(15, r.BT)
	if r.Unknown2 != nil {
		e.WriteMessage(16, EncodeReportTwoSixteen(r.Unknown2))
	}
	e.WriteInt64(18, int64(r.DPI))
	e.WriteInt64(20, int64(r.BatteryCapacity))
	e.WriteInt64(21, int64(r.Unknown3))
	e.WriteInt64(22, int64(r.Unknown4))
	e.WriteInt64(23, int64(r.Unknown5))
	e.WriteInt64(24, int64(r.Unknown6))
	e.WriteInt64(25, int64(r.Unknown7))
	e.WriteInt64(26, int64(r.Unknown8))
	e.WriteInt64(28, int64(r.Unknown7_1))
	e.WriteString(29, r.Notset2)
	e.WriteString(30, r.DeviceBrand)
	e.WriteString(31, r.ChipModel)
	e.WriteString(32, r.Feature)
	e.WriteString(33, r.Libs)
	e.WriteString(34, r.Unknown9)
	e.WriteString(35, r.Notset3)
	e.WriteString(36, r.Notset4)
	e.WriteString(37, r.Notset5)
	e.WriteString(38, r.Notset6)
	e.WriteString(39, r.Notset7)
	e.WriteString(40, r.DeviceSecurity)
	e.WriteInt64(41, int64(r.OSMin))
	e.WriteString(42, r.STHW)
	e.WriteInt64(43, int64(r.S3))
	e.WriteString(44, r.Notset8)
	e.WriteString(45, r.Notset9)
	e.WriteString(46, r.Notset10)
	return e.Bytes()
}

// EncodeReportThree 编码ReportThree消息
func EncodeReportThree(r *ReportThree) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, r.Token)
	e.WriteString(2, r.DeviceID)
	e.WriteString(3, r.InstallID)
	e.WriteString(4, r.Notset1)
	e.WriteString(5, r.Notset2)
	e.WriteString(6, r.Notset3)
	e.WriteString(7, r.Notset4)
	e.WriteString(8, r.Notset5)
	e.WriteString(9, r.OpenUDID)
	e.WriteString(10, r.OpenUDID1)
	e.WriteString(11, r.Notset6)
	e.WriteString(12, r.Notset7)
	e.WriteString(13, r.Notset8)
	e.WriteString(14, r.RequestID)
	e.WriteString(16, r.Unknown1)
	return e.Bytes()
}

// EncodeReportFour 编码ReportFour消息
func EncodeReportFour(r *ReportFour) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(r.Unknown1))
	e.WriteInt64(2, int64(r.Unknown2))
	e.WriteString(3, r.PackageName)
	e.WriteString(4, r.Notset1)
	e.WriteString(5, r.PackageName1)
	e.WriteString(6, r.Aid)
	e.WriteString(7, r.UpdateVersionCode)
	e.WriteString(8, r.Notset2)
	e.WriteString(9, r.Notset3)
	e.WriteString(10, r.Unknown3)
	e.WriteString(11, r.Channel)
	e.WriteString(12, r.SdkVersionStr)
	e.WriteString(13, r.ClientRegion)
	e.WriteString(14, r.Unknown4)
	e.WriteString(15, r.Notset4)
	e.WriteInt64(18, int64(r.S1))
	e.WriteInt64(19, int64(r.Unknown5))
	e.WriteInt64(20, int64(r.Unknown6))
	e.WriteInt64(22, int64(r.Unknown22))
	e.WriteInt64(23, int64(r.Unknown7))
	e.WriteInt64(24, int64(r.Unknown8))
	e.WriteString(26, r.Notset5)
	e.WriteString(27, r.Notset6)
	e.WriteString(28, r.Notset7)
	e.WriteInt64(29, int64(r.S2))
	e.WriteInt64(30, int64(r.Unknown30))
	e.WriteString(31, r.Notset8)
	e.WriteInt64(34, int64(r.S3))
	e.WriteInt64(35, int64(r.S4))
	e.WriteInt64(36, int64(r.S5))
	e.WriteInt64(37, int64(r.S6))
	e.WriteInt64(38, int64(r.Unknown9))
	return e.Bytes()
}

// EncodeReportFive 编码ReportFive消息
func EncodeReportFive(r *ReportFive) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(r.Unknown1))
	return e.Bytes()
}

// EncodeReportSixOne 编码ReportSixOne消息
func EncodeReportSixOne(r *ReportSixOne) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	if r.Unknown1 != 0 {
		e.WriteInt32(12, int32(r.Unknown1))
	}
	return e.Bytes()
}

// EncodeReportSix 编码ReportSix消息
func EncodeReportSix(r *ReportSix) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, r.BuildFingerprint)
	e.WriteString(2, r.Timezone)
	e.WriteString(3, r.Language)
	e.WriteString(4, r.OSVersion)
	e.WriteString(5, r.OS)
	e.WriteString(6, r.Notset1)
	e.WriteString(7, r.Notset2)
	e.WriteString(8, r.CPUABI)
	e.WriteInt64(9, int64(r.Unknown1))
	e.WriteInt64(10, int64(r.Unknown2))
	e.WriteInt64(11, int64(r.Unknown3))
	e.WriteInt64(12, int64(r.OSAPI))
	e.WriteInt64(13, int64(r.Unknown4))
	e.WriteInt64(14, int64(r.S1))
	e.WriteInt64(15, int64(r.Unknown5))
	e.WriteString(16, r.Notset3)
	e.WriteInt64(17, int64(r.Unknown6))
	e.WriteInt64(18, int64(r.Dat))
	if r.ReportSixOne != nil {
		e.WriteMessage(19, EncodeReportSixOne(r.ReportSixOne))
	}
	return e.Bytes()
}

// EncodeReportSeven 编码ReportSeven消息
func EncodeReportSeven(r *ReportSeven) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, r.Notset1)
	e.WriteString(2, r.Notset2)
	e.WriteString(3, r.LocalIP)
	e.WriteString(4, r.Notset3)
	e.WriteString(5, r.Internet)
	e.WriteInt64(7, int64(r.Unknown1))
	e.WriteString(8, r.Notset4)
	for _, gateway := range r.Gateway {
		e.WriteString(9, gateway)
	}
	e.WriteString(11, r.GIP)
	e.WriteString(12, r.Type)
	e.WriteString(13, r.Notset5)
	e.WriteInt64(14, int64(r.Unknown2))
	return e.Bytes()
}

// EncodeReportEight 编码ReportEight消息
func EncodeReportEight(r *ReportEight) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, r.Notset1)
	e.WriteInt64(2, int64(r.Unknown1))
	e.WriteInt64(3, int64(r.S1))
	e.WriteInt64(4, int64(r.S2))
	e.WriteString(5, r.Notset2)
	e.WriteString(6, r.Notset3)
	e.WriteString(7, r.Notset4)
	e.WriteInt64(8, int64(r.Unknown8))
	e.WriteInt64(9, int64(r.Unknown9))
	e.WriteInt64(11, int64(r.Unknown11))
	e.WriteInt64(12, int64(r.S12))
	e.WriteInt64(13, int64(r.S13))
	e.WriteString(14, r.Notset14)
	e.WriteString(15, r.Unknown15)
	e.WriteInt64(16, int64(r.Unknown16))
	e.WriteInt64(17, int64(r.Unknown17))
	e.WriteInt64(19, int64(r.S19))
	e.WriteInt64(20, int64(r.S20))
	e.WriteInt64(21, int64(r.S21))
	e.WriteInt64(22, int64(r.Unknown22))
	e.WriteString(23, r.Notset23)
	e.WriteString(24, r.Notset24)
	e.WriteString(25, r.Notset25)
	e.WriteString(26, r.Unknown26)
	e.WriteInt64(27, int64(r.S27))
	e.WriteInt64(30, int64(r.Unknown30))
	e.WriteInt64(31, int64(r.S31))
	e.WriteString(32, r.Notset32)
	e.WriteInt64(33, int64(r.Unknown33))
	e.WriteString(34, r.Unknwon34)
	e.WriteInt64(35, int64(r.S35))
	e.WriteString(36, r.Unknown36)
	e.WriteString(37, r.DeviceBrand)
	e.WriteString(38, r.ReleaseKey)
	e.WriteString(39, r.ReleaseKeyNum)
	e.WriteString(40, r.Fingerprint)
	e.WriteInt64(41, int64(r.Unknown41))
	e.WriteInt64(42, int64(r.Unknown42))
	e.WriteString(45, r.BD)
	for _, v := range r.Unknown47 {
		e.WriteString(47, v)
	}
	e.WriteInt64(49, int64(r.Unknown49))
	e.WriteInt64(51, int64(r.Unknown51))
	e.WriteString(52, r.Notset52)
	e.WriteInt64(53, int64(r.S53))
	e.WriteInt64(54, int64(r.S54))
	e.WriteInt64(55, int64(r.S55))
	e.WriteString(56, r.Notset56)
	e.WriteInt64(57, int64(r.S57))
	e.WriteInt64(58, int64(r.S58))
	e.WriteInt64(59, int64(r.S59))
	e.WriteInt64(60, int64(r.S60))
	e.WriteInt64(61, int64(r.Unknown61))
	e.WriteInt64(62, int64(r.Unknown62))
	e.WriteInt64(63, int64(r.Unknown63))
	e.WriteInt64(66, int64(r.Unknown66))
	e.WriteInt64(67, int64(r.S67))
	e.WriteInt64(68, int64(r.S68))
	e.WriteInt64(69, int64(r.SS69))
	e.WriteInt64(70, int64(r.SS70))
	e.WriteInt64(71, int64(r.SS71))
	e.WriteInt64(72, int64(r.SS72))
	e.WriteInt64(73, int64(r.S73))
	e.WriteInt64(74, int64(r.Unknown74))
	return e.Bytes()
}

// EncodeReportNine 编码ReportNine消息
func EncodeReportNine(r *ReportNine) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, r.Notset1)
	e.WriteString(2, r.Notset2)
	e.WriteString(3, r.Notset3)
	e.WriteInt64(4, int64(r.S1))
	e.WriteInt64(5, int64(r.S2))
	return e.Bytes()
}

// EncodeReportTen 编码ReportTen消息
func EncodeReportTen(r *ReportTen) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteString(1, r.Unknown1)
	e.WriteString(2, r.Unknown2)
	e.WriteInt64(3, int64(r.S3))
	e.WriteString(4, r.Unknown4)
	e.WriteString(5, r.Unknown5)
	e.WriteInt64(6, int64(r.Unknown6))
	e.WriteInt64(7, int64(r.Unknown7))
	e.WriteString(8, r.Notset8)
	return e.Bytes()
}

// EncodeReportThirteen 编码ReportThirteen消息
func EncodeReportThirteen(r *ReportThirteen) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(r.S1))
	e.WriteString(2, r.Notset2)
	e.WriteString(3, r.Notset3)
	e.WriteString(4, r.Notset4)
	e.WriteString(5, r.Notset5)
	e.WriteString(6, r.Notset6)
	e.WriteString(7, r.Notset7)
	e.WriteString(8, r.Notset8)
	e.WriteString(9, r.Notset9)
	e.WriteString(10, r.Notset10)
	return e.Bytes()
}

// EncodeReportFourteen 编码ReportFourteen消息
func EncodeReportFourteen(r *ReportFourteen) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(r.Unknown1))
	e.WriteInt64(2, int64(r.Unknown2))
	e.WriteString(3, r.Notset3)
	e.WriteString(4, r.Notset4)
	e.WriteString(5, r.Notset5)
	e.WriteInt64(6, int64(r.Unknown3))
	e.WriteInt64(7, int64(r.S1))
	e.WriteString(8, r.Notset8)
	e.WriteString(9, r.Notset9)
	e.WriteString(10, r.Notset10)
	e.WriteString(11, r.Notset11)
	return e.Bytes()
}

// EncodeReportEncrypt 编码ReportEncrypt消息
func EncodeReportEncrypt(r *ReportEncrypt) []byte {
	if r == nil {
		return nil
	}
	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(r.Stime))
	if r.ReportTwo != nil {
		e.WriteMessage(2, EncodeReportTwo(r.ReportTwo))
	}
	if r.ReportThree != nil {
		e.WriteMessage(3, EncodeReportThree(r.ReportThree))
	}
	if r.ReportFour != nil {
		e.WriteMessage(4, EncodeReportFour(r.ReportFour))
	}
	if r.ReportFive != nil {
		e.WriteMessage(5, EncodeReportFive(r.ReportFive))
	}
	if r.ReportSix != nil {
		e.WriteMessage(6, EncodeReportSix(r.ReportSix))
	}
	if r.ReportSeven != nil {
		e.WriteMessage(7, EncodeReportSeven(r.ReportSeven))
	}
	if r.ReportEight != nil {
		e.WriteMessage(8, EncodeReportEight(r.ReportEight))
	}
	if r.ReportNine != nil {
		e.WriteMessage(9, EncodeReportNine(r.ReportNine))
	}
	if r.ReportTen != nil {
		e.WriteMessage(10, EncodeReportTen(r.ReportTen))
	}
	if r.ReportEleven != nil {
		e.WriteMessage(11, []byte{})
	}
	if r.ReportTwelve != nil {
		e.WriteMessage(12, []byte{})
	}
	if r.ReportThirteen != nil {
		e.WriteMessage(13, EncodeReportThirteen(r.ReportThirteen))
	}
	if r.ReportFourteen != nil {
		e.WriteMessage(14, EncodeReportFourteen(r.ReportFourteen))
	}
	return e.Bytes()
}

// MakeReportEncrypt 创建ReportEncrypt消息
func MakeReportEncrypt(
	deviceID string,
	installID string,
	createTime int64,
	sdkVersion int,
	token string,
	osVersion string,
	deviceModel string,
	deviceBrand string,
	cpuABI string,
	resolution string,
	dpi int,
	aid int,
	channel string,
	packageName string,
	secSDKVersion string,
	openUDID string,
	p2_37 string,
	p14_10 string,
) *ReportEncrypt {
	if channel == "" {
		channel = "samsung_store"
	}
	if packageName == "" {
		packageName = "com.zhiliaoapp.musically"
	}
	if openUDID == "" {
		openUDID = installID
	}
	_ = resolution
	hasIntegrity := p2_37 != ""
	notset5 := "!notset!"
	notset6 := "!notset!"
	if hasIntegrity {
		notset5 = p2_37
		notset6 = "1876881764483077"
	}
	reportFourUnknown3 := "GzkdHUU5NTE1ORMxRZmd"
	if hasIntegrity {
		reportFourUnknown3 = "IyU1NTEz"
	}
	unknownFiveValue := uint64(41 << 1)
	if hasIntegrity {
		unknownFiveValue = 80 << 1
	}
	unknownSixValue := uint64(1 << 1)
	if hasIntegrity {
		unknownSixValue = 2 << 1
	}
	unknownEightValue := uint64(17 << 1)
	if hasIntegrity {
		unknownEightValue = 40 << 1
	}
	unknownNineValue := uint64(824158252550848529 << 1)
	if hasIntegrity {
		unknownNineValue = 7620106533837209648
	}
	unknown22 := uint64(0)
	if hasIntegrity {
		unknown22 = 2
	}
	unknown30 := uint64(0)
	if hasIntegrity {
		unknown30 = 1 << 1
	}

	two := &ReportTwo{
		UnknownUtime:    1761976310765 << 1,
		Unknown1:        1400287788 << 1,
		NCPU:            8 << 1,
		PinlvCPUMax:     "1803000",
		PinlvCPUMin:     "300000",
		S1:              1999997,
		S2:              1999997,
		Chuliqi:         "fp asimd evtstrm aes pmull sha1 sha2 crc32 atomics fphp asimdhp cpuid asimdrdm lrcpc dcpop asimddp",
		Netset1:         "!notset!",
		DeviceType:      deviceModel,
		TwoName1:        deviceModel,
		TwoName2:        deviceBrand,
		TwoName3:        channel,
		BT:              "slider-15.3-13239612",
		Unknown2:        &ReportTwoSixteen{Unknown6: 3472332702763464752},
		DPI:             uint64(dpi) << 1,
		BatteryCapacity: 4614 << 1,
		Unknown3:        10,
		Unknown4:        2,
		Unknown5:        2,
		Unknown6:        7958913024 << 1,
		Unknown7:        118396899328 << 1,
		Unknown8:        18274320384 << 1,
		Unknown7_1:      118396899328 << 1,
		Notset2:         "!notset!",
		DeviceBrand:     deviceBrand,
		ChipModel:       cpuABI,
		Feature:         reportTwoFeature,
		Libs:            reportTwoLibs,
		Unknown9:        "13.8",
		Notset3:         "!notset!",
		Notset4:         "!notset!",
		Notset5:         notset5,
		Notset6:         notset6,
		Notset7:         "!notset!",
		DeviceSecurity:  deviceSecurityJSON,
		OSMin:           31 << 1,
		STHW:            stHWJSON,
		S3:              1999997,
		Notset8:         "!notset!",
		Notset9:         "!notset!",
		Notset10:        "!notset!",
	}

	three := &ReportThree{
		Token:     token,
		DeviceID:  deviceID,
		InstallID: installID,
		Notset1:   "!notset!",
		Notset2:   "!notset!",
		Notset3:   "!notset!",
		Notset4:   "!notset!",
		Notset5:   "!notset!",
		Notset6:   "!notset!",
		Notset7:   "!notset!",
		Notset8:   "!notset!",
		OpenUDID:  openUDID,
		OpenUDID1: openUDID,
		RequestID: randomUUID(),
		Unknown1:  GenerateFakeMediadrmID(),
	}

	four := &ReportFour{
		Unknown1:          1763202546 << 1,
		Unknown2:          uint64(createTime*1000) << 1,
		PackageName:       packageName,
		Notset1:           "!notset!",
		PackageName1:      packageName,
		Aid:               fmt.Sprintf("%d", aid),
		UpdateVersionCode: secSDKVersion,
		Notset2:           "!notset!",
		Notset3:           "!notset!",
		Unknown3:          reportFourUnknown3,
		Channel:           channel,
		SdkVersionStr:     fmt.Sprintf("v%02d.02.02-alpha.12-ov-android", sdkVersion),
		ClientRegion:      "ov",
		Unknown4:          "inhouse",
		Notset4:           "!notset!",
		S1:                1999997,
		Unknown5:          unknownFiveValue,
		Unknown6:          unknownSixValue,
		Unknown22:         unknown22,
		Unknown7:          1 << 1,
		Unknown8:          unknownEightValue,
		Notset5:           "!notset!",
		Notset6:           "!notset!",
		Notset7:           "!notset!",
		S2:                1999997,
		Unknown30:         unknown30,
		Notset8:           "!notset!",
		S3:                1999997,
		S4:                1999997,
		S5:                1999997,
		S6:                1999997,
		Unknown9:          unknownNineValue,
	}

	five := &ReportFive{
		Unknown1: 10 << 1,
	}

	six := &ReportSix{
		BuildFingerprint: fmt.Sprintf("%s/%s:%s/BP1A.250505.005/13277524:user/release-keys", strings.ToLower(deviceBrand), strings.ToLower(deviceModel), osVersion),
		Timezone:         "America/New_York,-5",
		Language:         "en_",
		OSVersion:        osVersion,
		OS:               "android",
		Notset1:          "!notset!",
		Notset2:          "!notset!",
		CPUABI:           cpuABI,
		Unknown1:         2 << 1,
		Unknown2:         1 << 1,
		Unknown3:         1,
		OSAPI:            35 << 1,
		Unknown4:         1,
		S1:               1999997,
		Unknown5:         8 << 1,
		Notset3:          "!notset!",
		Unknown6:         2371331800 << 1,
		Dat:              uint64(createTime) << 1,
		ReportSixOne: &ReportSixOne{
			Unknown1: 1398091118,
		},
	}

	seven := &ReportSeven{
		Notset1:  "!notset!",
		Notset2:  "!notset!",
		LocalIP:  "192.168.15.54",
		Notset3:  "!notset!",
		Internet: reportSevenInternet,
		Unknown1: 1,
		Notset4:  "!notset!",
		Gateway:  []string{"192.168.15.26", "0.0.0.0"},
		GIP:      "192.168.15.26",
		Type:     "wlan0",
		Notset5:  "!notset!",
	}
	if hasIntegrity {
		seven.Unknown2 = 2 << 1
	} else {
		seven.Unknown2 = 1 << 1
	}

	eight := &ReportEight{
		Notset1:       "!notset!",
		Unknown8:      1054 << 1,
		Unknown9:      10302 << 1,
		Unknown11:     1 << 1,
		S12:           1999997,
		S13:           1999997,
		Notset14:      "!notset!",
		Unknown15:     "d79f7cb8509a5e7e71c4f2afcfb75ea8c87177ca",
		Unknown16:     1 << 1,
		S19:           1999997,
		S20:           1999997,
		S21:           1999997,
		Unknown22:     2,
		Notset23:      "!notset!",
		Notset24:      "!notset!",
		Notset25:      "!notset!",
		S27:           1999997,
		Unknown30:     0,
		S31:           1999997,
		Notset32:      "!notset!",
		Unknown33:     1 << 1,
		S35:           1999997,
		DeviceBrand:   deviceBrand,
		ReleaseKey:    "release-keys",
		ReleaseKeyNum: "13277524",
		Fingerprint:   fmt.Sprintf("%s-user %s", packageName, time.Now().Format("200601")),
		Unknown41:     1,
		Unknown42:     1,
		BD:            "bd",
		Unknown47: []string{
			"eWdzX2Vpc0VdZX1tcVNFER05G0UbJSUjRRsfKSE5HwmBnZk=",
			"U2Fbc2lfgaUzORM5pREdMR+lm6U9JSGnDyspIyk5JTkbG6chER0pPTkjIwmlPTk9KzE=",
		},
		Unknown51: 1,
		Notset52:  "!notset!",
		S53:       1999997,
		S54:       1999997,
		S55:       1999997,
		Notset56:  "!notset!",
		S57:       1999997,
		S58:       1999997,
		S59:       1999997,
		S60:       1999997,
		Unknown61: 1,
		Unknown62: 1,
		Unknown63: 1,
		S67:       1999997,
		S68:       1999997,
		SS69:      1999997,
		SS70:      1999997,
		SS71:      1999997,
		SS72:      1999997,
		S73:       1999997,
	}
	if hasIntegrity {
		eight.Unknown17 = 0
		eight.Unknown66 = 567391857 << 1
		eight.Unknown74 = 4855976879408029395 << 1
	} else {
		eight.Unknown17 = 1 << 1
		eight.Unknown49 = 1
		eight.Unknown66 = 864448950 << 1
		eight.Unknown74 = 1166690942474438016 << 1
	}

	nine := &ReportNine{
		Notset1: "!notset!",
		Notset2: "!notset!",
		Notset3: "!notset!",
		S1:      1999997,
		S2:      1999997,
	}

	ten := &ReportTen{
		Unknown1: buildReportTenUnknown1(createTime, hasIntegrity),
		Unknown2: "{}",
		S3:       1999997,
		Unknown4: reportTenUnknown4,
		Unknown5: "m5ubm5ubm5ubm5ubm5ubmw==",
		Unknown6: 2 << 1,
		Unknown7: 503 << 1,
		Notset8:  "!notset!",
	}

	thirteen := &ReportThirteen{
		S1:       uint64(rand.Intn(100)),
		Notset2:  "!notset!",
		Notset3:  "!notset!",
		Notset4:  "!notset!",
		Notset5:  "!notset!",
		Notset6:  "!notset!",
		Notset7:  "!notset!",
		Notset8:  "!notset!",
		Notset9:  "!notset!",
		Notset10: "!notset!",
	}

	fourteen := &ReportFourteen{
		Unknown1: 1 << 1,
		Unknown2: 1 << 1,
		Notset3:  "!notset!",
		Notset4:  "!notset!",
		Notset5:  "!notset!",
		S1:       1999997,
		Notset8:  "!notset!",
		Notset9:  "!notset!",
		Notset11: "!notset!",
	}
	if p14_10 == "" {
		fourteen.Unknown3 = 1
		fourteen.Notset10 = "!notset!"
	} else {
		fourteen.Unknown3 = uint64(createTime) << 1
		fourteen.Notset10 = p14_10
	}

	return &ReportEncrypt{
		Stime:          uint64(createTime),
		ReportTwo:      two,
		ReportThree:    three,
		ReportFour:     four,
		ReportFive:     five,
		ReportSix:      six,
		ReportSeven:    seven,
		ReportEight:    eight,
		ReportNine:     nine,
		ReportTen:      ten,
		ReportEleven:   &Empty{},
		ReportTwelve:   &Empty{},
		ReportThirteen: thirteen,
		ReportFourteen: fourteen,
	}
}

// MakeReportRequest 创建ReportRequest消息并序列化
// reportEncryptHex: 已加密后的report_encrypt字段(hex字符串)
// utime: 当前毫秒时间戳
func MakeReportRequest(reportEncryptHex string, utime int64) (string, error) {
	reportEncryptBytes, err := hex.DecodeString(reportEncryptHex)
	if err != nil {
		return "", err
	}

	e := NewProtobufEncoder()
	e.WriteInt64(1, int64(538969122<<1))
	e.WriteInt64(2, 2)
	e.WriteBytes(4, reportEncryptBytes)
	e.WriteInt64(5, utime<<1)

	return hex.EncodeToString(e.Bytes()), nil
}

// DecodeReportDecrypt 解码ReportDecrypt消息
func DecodeReportDecrypt(data []byte) (*ReportDecrypt, error) {
	d := NewProtobufDecoder(data)
	result := &ReportDecrypt{}

	for d.HasMore() {
		fieldNum, wireType, err := d.ReadTag()
		if err != nil {
			break
		}

		switch fieldNum {
		case 1:
			result.Code, _ = d.ReadInt32()
		case 2:
			result.Message, _ = d.ReadString()
		default:
			d.Skip(wireType)
		}
	}

	return result, nil
}

// MakeReportDecrypt 解析ReportDecrypt消息
func MakeReportDecrypt(hexData string) (*ReportDecrypt, error) {
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, err
	}
	return DecodeReportDecrypt(data)
}

// DecodeReprotResponse 解码ReprotResponse消息
func DecodeReprotResponse(data []byte) (*ReprotResponse, error) {
	d := NewProtobufDecoder(data)
	result := &ReprotResponse{}

	for d.HasMore() {
		fieldNum, wireType, err := d.ReadTag()
		if err != nil {
			break
		}

		switch fieldNum {
		case 1:
			innerData, _ := d.ReadBytes()
			result.Report, _ = DecodeReportDecrypt(innerData)
		default:
			d.Skip(wireType)
		}
	}

	return result, nil
}

// MakeReportResponse 解析ReprotResponse消息
func MakeReportResponse(hexData string) (*ReprotResponse, error) {
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, err
	}
	return DecodeReprotResponse(data)
}
