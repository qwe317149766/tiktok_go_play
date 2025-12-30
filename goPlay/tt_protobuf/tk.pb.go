package tt_protobuf

// tk.pb.go mirrors definitions in tt_protobuf/tk.proto. It is intentionally
// lightweight and keeps a handful of legacy placeholder fields to avoid
// breaking the existing helper code. Prefer using the named fields that match
// the proto schema.

type Argus struct {
	Magic          uint32
	Version        uint32
	Rand           uint64
	MsAppID        string
	DeviceID       string
	LicenseID      string
	AppVersion     string
	SdkVersionStr  string
	SdkVersion     uint32
	EnvCode        []byte
	Platform       uint32
	CreateTime     uint64
	BodyHash       []byte
	QueryHash      []byte
	ActionRecord   *ActionRecord
	SecDeviceToken string
	IsAppLicense   uint64
	PskHash        []byte
	PskCalHash     []byte
	PskVersion     string
	CallType       uint32
	ChannelInfo    *ChannelInfo
	Seed           string
	ExtType        uint32
	ExtraInfo      []*ExtraInfo
	Unknown28      uint32
	Unknown29      uint32
	Unknown30      uint32
	Unknown31      uint64
	Unknown32      []byte
	Unknown33      uint32
}

type ExtraInfo struct {
	Algorithm     uint32
	AlgorithmData []byte
}

type ChannelInfo struct {
	PhoneInfo          string
	MetasecConstant    uint32
	Channel            string
	AppVersionConstant uint32
}

type ActionRecord struct {
	SignCount          uint32
	ReportCount        uint32
	SettingCount       uint32
	ReportFailCount    uint32
	ReportSuccessCount uint32
	ActionIncremental  uint32
	AppLaunchTime      uint32
	SeedType           uint32
}

type SeedEncrypt struct {
	Session    string
	DeviceID   string
	OS         string
	SDKVersion string
}

type SeedRequest struct {
	S1      uint64
	S2      uint64
	S3      uint64
	Encrypt []byte
	Utime   uint64
}

type SeedDecrypt struct {
	Seed      string
	ExtraInfo *SeedInfo
}

type SeedInfo struct {
	Algorithm string
}

type SeedResponse struct {
	S1               uint64
	S2               uint64
	S3               uint64
	SeedDecrypt      string
	SeedDecryptBytes []byte
}

type TaskEncrypt struct {
	Aid         string
	DeviceID    string
	Platform    string
	VersionName string
	BuildName   string
	Type        uint32
}

type TaskRequest struct {
	S1      uint64
	S2      uint64
	S3      uint64
	Encrypt []byte
	Utime   uint64
}

type DynReportEncrypt struct {
	Aid         string
	DeviceID    string
	Platform    string
	VersionName string
	Unknown2    string
	Sig         *DynReportSignature
}

type DynReportSignature struct {
	Alg   uint32
	Key   string
	Extra string
}

type DynReportRequest struct {
	S1      uint64
	S2      uint64
	S3      uint64
	Encrypt []byte
	Utime   uint64
}

type TokenRequest struct {
	S1           uint64
	S2           uint64
	S3           uint64
	TokenEncrypt []byte
	Utime        uint64
}

type TokenEncrypt struct {
	One        *TokenEncryptOne
	LastToken  string
	OS         string
	SdkVer     string
	SdkVerCode uint64
	MsAppID    string
	AppVersion string
	DeviceID   string
	Two        *TokenEncryptTwo
	Stable1    uint64
	Unknown2   string
	Notset1    string
	Stable2    uint64

	LegacyTokenEncrypt
}

type TokenEncrypt_One = TokenEncryptOne

type TokenEncryptOne struct {
	Notset1            string
	Changshang         string
	Xinghao            string
	Notset2            string
	OS                 string
	OSVersion          string
	TokenEncryptOneOne *TokenEncryptOneOne
	DensityDPI         uint32
	BuildID            string
	OSBuildTime        uint64
	AppLanguage        string
	TimeZone           string
	Unknown2           uint64
	Unknown3           uint64
	Unknown4           uint64
	Stable1            uint64
	Stable2            uint64
	Unknown5           uint64
	Notset3            string
	Notset4            string
	AndroidID          string
	Notset5            string
	Notset6            string
	MediaDrm           string
	LaunchTime         uint64
	BootID             string
	Unknown6           uint64
	Notset7            string
	Stable3            uint64
	Stable4            uint64
	Notset8            string
	DefaultGateway     string
	IPDNS              string
	Notset9            string
	Netset10           string
	Stable5            uint64
	Stable6            uint64
	Notset11           string
	Notset12           string
	IPArray            string
	Stable7            uint64
	Stable8            uint64
	Notset13           string
	ExpiredTime        uint64
	SendTime           uint64
	InstallPath        string
	OSAPI              uint64
	Stable9            uint64
	Notset14           string
	Stable10           uint64
	Stable11           uint64
	Notset15           string
	Notset16           string
	Notset17           string
	Stable12           uint64

	LegacyTokenEncryptOne
}

type TokenEncrypt_One_One = TokenEncryptOneOne

type TokenEncryptOneOne struct {
	Unknown1 uint64

	LegacyTokenEncryptOneOne
}

type TokenEncrypt_Two = TokenEncryptTwo

type TokenEncryptTwo struct {
	S6 []uint64

	LegacyTokenEncryptTwo
}

type LegacyTokenEncrypt struct {
	Field1  int32
	Field2  int32
	Field3  string
	Field4  int64
	Field5  int64
	Field6  int32
	Field7  int32
	Field8  *TokenEncryptOne
	Field9  string
	Field10 int32
	Field11 int64
	Field12 string
}

type LegacyTokenEncryptOne struct {
	Field1  *TokenEncryptOneOne
	Field2  *TokenEncryptTwo
	Field3  string
	Field4  int64
	Field5  int64
	Field6  string
	Field7  string
	Field8  string
	Field9  string
	Field10 string
	Field11 int32
	Field12 string
	Field13 string
	Field14 string
	Field15 int32
	Field16 string
	Field17 int32
	Field18 string
}

type LegacyTokenEncryptOneOne struct {
	Field1  int32
	Field2  int32
	Field3  string
	Field4  string
	Field5  string
	Field6  int32
	Field7  string
	Field8  string
	Field9  string
	Field10 string
}

type LegacyTokenEncryptTwo struct {
	Field1  int32
	Field2  string
	Field3  string
	Field4  string
	Field5  int32
	Field6  int32
	Field7  string
	Field8  string
	Field9  int32
	Field10 int64
	Field11 string
}

type TokenDecrypt struct {
	Token      string
	S2         uint64
	ExpireTime int64
}

type TokenResponse struct {
	S1                uint64
	S2                uint64
	S3                uint64
	TokenDecryptBytes []byte
	TokenDecrypt      string
}

type ReportRequest struct {
	S1            uint64
	S2            uint64
	S3            uint64
	ReportEncrypt []byte
	Utime         uint64
}

type ReportEncrypt struct {
	Stime          uint64
	ReportTwo      *ReportTwo
	ReportThree    *ReportThree
	ReportFour     *ReportFour
	ReportFive     *ReportFive
	ReportSix      *ReportSix
	ReportSeven    *ReportSeven
	ReportEight    *ReportEight
	ReportNine     *ReportNine
	ReportTen      *ReportTen
	ReportEleven   *Empty
	ReportTwelve   *Empty
	ReportThirteen *ReportThirteen
	ReportFourteen *ReportFourteen

	LegacyReportEncrypt
}

type ReportTwo struct {
	UnknownUtime    uint64
	Unknown1        uint64
	NCPU            uint64
	PinlvCPUMax     string
	PinlvCPUMin     string
	S1              uint64
	S2              uint64
	Chuliqi         string
	Netset1         string
	DeviceType      string
	TwoName1        string
	TwoName2        string
	TwoName3        string
	BT              string
	Unknown2        *ReportTwoSixteen
	DPI             uint64
	BatteryCapacity uint64
	Unknown3        uint64
	Unknown4        uint64
	Unknown5        uint64
	Unknown6        uint64
	Unknown7        uint64
	Unknown8        uint64
	Unknown7_1      uint64
	Notset2         string
	DeviceBrand     string
	ChipModel       string
	Feature         string
	Libs            string
	Unknown9        string
	Notset3         string
	Notset4         string
	Notset5         string
	Notset6         string
	Notset7         string
	DeviceSecurity  string
	OSMin           uint64
	STHW            string
	S3              uint64
	Notset8         string
	Notset9         string
	Notset10        string

	LegacyReportTwo
}

type ReportTwoSixteen struct {
	Unknown6 uint64

	LegacyReportTwoSixteen
}

type ReportThree struct {
	Token     string
	DeviceID  string
	InstallID string
	Notset1   string
	Notset2   string
	Notset3   string
	Notset4   string
	Notset5   string
	OpenUDID  string
	OpenUDID1 string
	Notset6   string
	Notset7   string
	Notset8   string
	RequestID string
	Unknown1  string

	LegacyReportThree
}

type ReportFour struct {
	Unknown1          uint64
	Unknown2          uint64
	PackageName       string
	Notset1           string
	PackageName1      string
	Aid               string
	UpdateVersionCode string
	Notset2           string
	Notset3           string
	Unknown3          string
	Channel           string
	SdkVersionStr     string
	ClientRegion      string
	Unknown4          string
	Notset4           string
	S1                uint64
	Unknown5          uint64
	Unknown6          uint64
	Unknown22         uint64
	Unknown7          uint64
	Unknown8          uint64
	Notset5           string
	Notset6           string
	Notset7           string
	S2                uint64
	Unknown30         uint64
	Notset8           string
	S3                uint64
	S4                uint64
	S5                uint64
	S6                uint64
	Unknown9          uint64

	LegacyReportFour
}

type ReportFive struct {
	Unknown1 uint64

	LegacyReportFive
}

type ReportSix struct {
	BuildFingerprint string
	Timezone         string
	Language         string
	OSVersion        string
	OS               string
	Notset1          string
	Notset2          string
	CPUABI           string
	Unknown1         uint64
	Unknown2         uint64
	Unknown3         uint64
	OSAPI            uint64
	Unknown4         uint64
	S1               uint64
	Unknown5         uint64
	Notset3          string
	Unknown6         uint64
	Dat              uint64
	ReportSixOne     *ReportSixOne

	LegacyReportSix
}

type ReportSixOne struct {
	Unknown1 uint32

	LegacyReportSixOne
}

type ReportSeven struct {
	Notset1  string
	Notset2  string
	LocalIP  string
	Notset3  string
	Internet string
	Unknown1 uint64
	Notset4  string
	Gateway  []string
	GIP      string
	Type     string
	Notset5  string
	Unknown2 uint64

	LegacyReportSeven
}

type ReportEight struct {
	Notset1       string
	Unknown1      uint64
	S1            uint64
	S2            uint64
	Notset2       string
	Notset3       string
	Notset4       string
	Unknown8      uint64
	Unknown9      uint64
	Unknown11     uint64
	S12           uint64
	S13           uint64
	Notset14      string
	Unknown15     string
	Unknown16     uint64
	Unknown17     uint64
	S19           uint64
	S20           uint64
	S21           uint64
	Unknown22     uint64
	Notset23      string
	Notset24      string
	Notset25      string
	Unknown26     string
	S27           uint64
	Unknown30     uint64
	S31           uint64
	Notset32      string
	Unknown33     uint64
	Unknwon34     string
	S35           uint64
	Unknown36     string
	DeviceBrand   string
	ReleaseKey    string
	ReleaseKeyNum string
	Fingerprint   string
	Unknown41     uint64
	Unknown42     uint64
	BD            string
	Unknown47     []string
	Unknown49     uint64
	Unknown51     uint64
	Notset52      string
	S53           uint64
	S54           uint64
	S55           uint64
	Notset56      string
	S57           uint64
	S58           uint64
	S59           uint64
	S60           uint64
	Unknown61     uint64
	Unknown62     uint64
	Unknown63     uint64
	Unknown66     uint64
	S67           uint64
	S68           uint64
	SS69          uint64
	SS70          uint64
	SS71          uint64
	SS72          uint64
	S73           uint64
	Unknown74     uint64

	LegacyReportEight
}

type ReportNine struct {
	Notset1 string
	Notset2 string
	Notset3 string
	S1      uint64
	S2      uint64

	LegacyReportNine
}

type ReportTen struct {
	Unknown1 string
	Unknown2 string
	S3       uint64
	Unknown4 string
	Unknown5 string
	Unknown6 uint64
	Unknown7 uint64
	Notset8  string

	LegacyReportTen
}

type ReportThirteen struct {
	S1       uint64
	Notset2  string
	Notset3  string
	Notset4  string
	Notset5  string
	Notset6  string
	Notset7  string
	Notset8  string
	Notset9  string
	Notset10 string

	LegacyReportThirteen
}

type ReportFourteen struct {
	Unknown1 uint64
	Unknown2 uint64
	Notset3  string
	Notset4  string
	Notset5  string
	Unknown3 uint64
	S1       uint64
	Notset8  string
	Notset9  string
	Notset10 string
	Notset11 string

	LegacyReportFourteen
}

type LegacyReportEncrypt struct {
	Field1  int32
	Field2  *ReportTwo
	Field3  *ReportThree
	Field4  *ReportFour
	Field5  *ReportFive
	Field6  *ReportSix
	Field7  *ReportSeven
	Field8  *ReportEight
	Field9  *ReportNine
	Field10 *ReportTen
	Field11 int32
	Field12 int32
	Field13 *ReportThirteen
	Field14 *ReportFourteen
}

type LegacyReportTwo struct {
	Field1  string
	Field2  string
	Field3  string
	Field4  string
	Field5  string
	Field6  string
	Field7  string
	Field8  string
	Field9  string
	Field10 string
	Field11 string
	Field12 string
	Field13 string
	Field14 string
	Field15 string
	Field16 *ReportTwoSixteen
	Field17 string
}

type LegacyReportTwoSixteen struct {
	Field1 string
	Field2 int32
	Field3 int32
}

type LegacyReportThree struct {
	Field1 string
	Field2 string
	Field3 string
	Field4 string
	Field5 string
	Field6 int64
	Field7 int64
	Field8 int32
}

type LegacyReportFour struct {
	Field1 int64
	Field2 int64
	Field3 int64
	Field4 int64
	Field5 int32
}

type LegacyReportFive struct {
	Field1 string
	Field2 string
	Field3 string
	Field4 string
}

type LegacyReportSix struct {
	Field1 *ReportSixOne
}

type LegacyReportSixOne struct {
	Field1 string
	Field2 string
	Field3 string
	Field4 string
	Field5 string
}

type LegacyReportSeven struct {
	Field1 string
	Field2 int32
	Field3 int32
	Field4 int32
	Field5 int32
	Field6 int32
}

type LegacyReportEight struct {
	Field1 int32
}

type LegacyReportNine struct {
	Field1 string
}

type LegacyReportTen struct {
	Field1 int32
}

type LegacyReportThirteen struct {
	Field1 string
}

type LegacyReportFourteen struct {
	Field1 string
	Field2 string
}

type ReportDecrypt struct {
	Code    int32
	Message string
}

type ReprotResponse struct {
	Report *ReportDecrypt
}

type PrivateMessage struct {
	UnknownField1  int32
	Unknown2       int32
	Source         string
	Signature      string
	UnknownField5  int32
	UnknownField6  int32
	UnknownField7  string
	Details        *EventDetails
	DeviceID       string
	AppStore       string
	OS             string
	DeviceModel    string
	OSVersion      string
	AppVersion     string
	Metadata       []*KeyValuePair
	UnknownField18 uint32
	EmptyField21   string
}

type EventDetails struct {
	MessageContent *MessageInfo
}

type MessageInfo struct {
	CompositeID    string
	Type           int32
	ConversationID int64
	PayloadJSON    string
	Attributes     []*KeyValuePair
	AweType        int32
	MessageUUID    string
	EmptyField13   *Empty
	EmptyField14   *Empty
	EmptyField18   *Empty
	EmptyField19   *Empty
}

type KeyValuePair struct {
	Key   string
	Value string
}

type Empty struct{}

type CreateAConversation struct {
	CmdID               uint64
	SequenceID          uint64
	Type                string
	Signature           string
	Field5              uint64
	Field6              uint32
	Field7              string
	MessageInfo         *InnerMessage8
	DeviceID            string
	Channel             string
	OS                  string
	DeviceType          string
	OSVersion           string
	ManifestVersionCode string
	Headers             []*Header
	Field18             uint64
}

type Header struct {
	Key   string
	Value string
}

type InnerMessage8 struct {
	M609 *InnerMessage609
}

type InnerMessage609 struct {
	Type uint64
	ID   []uint64
}
