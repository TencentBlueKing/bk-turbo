package types

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// TraceType constants - equivalent to C++ enum
const (
	TraceTypeSessionAdded = iota
	TraceTypeSessionUpdate
	TraceTypeProcessAdded
	TraceTypeProcessExited
	TraceTypeProcessReturned
	TraceTypeFileBeginFetch
	TraceTypeFileEndFetch
	TraceTypeFileBeginStore
	TraceTypeFileEndStore
	TraceTypeSummary
	TraceTypeBeginWork
	TraceTypeEndWork
	TraceTypeString
	TraceTypeSessionSummary
	TraceTypeProcessEnvironmentUpdated
	TraceTypeSessionDisconnect
	TraceTypeProxyCreated
	TraceTypeProxyUsed
	TraceTypeFileFetchLight
	TraceTypeFileStoreLight
	TraceTypeStatusUpdate
	TraceTypeSessionNotification
	TraceTypeCacheBeginFetch
	TraceTypeCacheEndFetch
	TraceTypeCacheBeginWrite
	TraceTypeCacheEndWrite
	TraceTypeProgressUpdate
	TraceTypeRemoteExecutionDisabled
	TraceTypeFileFetchSize
	TraceTypeProcessBreadcrumbs
	TraceTypeWorkHint
	TraceTypeDriveUpdate
)

// TraceTypeNames maps trace type constants to their string names
var TraceTypeNames = map[uint8]string{
	TraceTypeSessionAdded:              "SessionAdded",
	TraceTypeSessionUpdate:             "SessionUpdate",
	TraceTypeProcessAdded:              "ProcessAdded",
	TraceTypeProcessExited:             "ProcessExited",
	TraceTypeProcessReturned:           "ProcessReturned",
	TraceTypeFileBeginFetch:            "FileBeginFetch",
	TraceTypeFileEndFetch:              "FileEndFetch",
	TraceTypeFileBeginStore:            "FileBeginStore",
	TraceTypeFileEndStore:              "FileEndStore",
	TraceTypeSummary:                   "Summary",
	TraceTypeBeginWork:                 "BeginWork",
	TraceTypeEndWork:                   "EndWork",
	TraceTypeString:                    "String",
	TraceTypeSessionSummary:            "SessionSummary",
	TraceTypeProcessEnvironmentUpdated: "ProcessEnvironmentUpdated",
	TraceTypeSessionDisconnect:         "SessionDisconnect",
	TraceTypeProxyCreated:              "ProxyCreated",
	TraceTypeProxyUsed:                 "ProxyUsed",
	TraceTypeFileFetchLight:            "FileFetchLight",
	TraceTypeFileStoreLight:            "FileStoreLight",
	TraceTypeStatusUpdate:              "StatusUpdate",
	TraceTypeSessionNotification:       "SessionNotification",
	TraceTypeCacheBeginFetch:           "CacheBeginFetch",
	TraceTypeCacheEndFetch:             "CacheEndFetch",
	TraceTypeCacheBeginWrite:           "CacheBeginWrite",
	TraceTypeCacheEndWrite:             "CacheEndWrite",
	TraceTypeProgressUpdate:            "ProgressUpdate",
	TraceTypeRemoteExecutionDisabled:   "RemoteExecutionDisabled",
	TraceTypeFileFetchSize:             "FileFetchSize",
	TraceTypeProcessBreadcrumbs:        "ProcessBreadcrumbs",
	TraceTypeWorkHint:                  "WorkHint",
	TraceTypeDriveUpdate:               "DriveUpdate",
}

// GUID represents a Windows GUID
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
	// Part1 uint64 // 低 8 字节
	// Part2 uint64 // 高 8 字节
}

// CasKey represents a content-addressable storage key
type CasKey [20]byte

var CasKeyZero = CasKey{}

// Process represents a traced process
type Process struct {
	ID              uint32
	ExitCode        uint32
	Start           uint64
	Stop            uint64
	Description     string
	ReturnedReason  string
	Breadcrumbs     string
	BitmapDirty     bool
	CacheFetch      bool
	IsRemote        bool
	CreateFilesTime uint64
	WriteFilesTime  uint64
	Stats           []byte
	LogLines        []ProcessLogLine
}

// ProcessLogLine represents a log line from a process
type ProcessLogLine struct {
	Text string
	Type uint8
}

// Processor represents a processor with processes
type Processor struct {
	Processes []Process
}

// SessionUpdate represents session update information
type SessionUpdate struct {
	Time            uint64
	Send            uint64
	Recv            uint64
	Ping            uint64
	MemAvail        uint64
	CPULoad         float32
	ConnectionCount uint8
}

// Drive represents a drive
type Drive struct {
	BusyHighest     uint8
	TotalReadCount  uint32
	TotalWriteCount uint32
	TotalReadBytes  uint64
	TotalWriteBytes uint64
	BusyPercent     []uint8
	ReadCount       []uint32
	WriteCount      []uint32
	ReadBytes       []uint64
	WriteBytes      []uint64
}

// FileTransfer represents a file transfer operation
type FileTransfer struct {
	Key   CasKey
	Size  uint64
	Hint  string
	Start uint64
	Stop  uint64
}

// Session represents a traced session
type Session struct {
	Name               string
	FullName           string
	Hyperlink          string
	ClientUID          GUID
	Processors         []Processor
	Updates            []uint64
	NetworkSend        []uint64
	NetworkRecv        []uint64
	Ping               []uint64
	MemAvail           []uint64
	CpuLoad            []float32
	ConnectionCount    []uint16
	ReconnectIndices   []uint32
	Summary            []string
	FetchedFilesActive map[CasKey]uint32
	FetchedFiles       []FileTransfer
	StoredFilesActive  map[CasKey]uint32
	StoredFiles        []FileTransfer
	Drives             map[uint8]Drive
	Notification       string
	FetchedFilesBytes  uint64
	StoredFilesBytes   uint64
	FetchedFilesCount  uint32
	StoredFilesCount   uint32
	MaxVisibleFiles    uint32
	FullNameWidth      uint32
	HighestSendPerS    float32
	HighestRecvPerS    float32
	IsReset            bool
	DisconnectTime     uint64
	PrevUpdateTime     uint64
	PrevSend           uint64
	PrevRecv           uint64
	MemTotal           uint64
	ProcessActiveCount uint32
	ProcessExitedCount uint32
	ProxyName          string
	ProxyCreated       bool
}

// WorkRecordLogEntry represents a work record log entry
type WorkRecordLogEntry struct
{
	Time uint64
	StartTime uint64
	Text string
	Count uint32
}

// WorkRecord represents a work record
type WorkRecord struct {
	Description  string
	Start        uint64
	Stop         uint64
	Entries 	 []WorkRecordLogEntry 
	BitmapOffset uint32
}

// WorkTrack represents a work track
type WorkTrack struct {
	Records []WorkRecord
}

// StatusUpdate represents a status update
type StatusUpdate struct {
	Text string
	Type uint8
	Link string
}

// CacheWrite represents a cache write operation
type CacheWrite struct {
	Start     uint64
	End       uint64
	Success   bool
	BytesSent uint64
}

// Timer represents a timer with time and count
type Timer struct {
	Time  uint64
	Count uint32
}

// TimeAndBytes represents a timer with time, count and bytes
type TimeAndBytes struct {
	Time  uint64
	Count uint32
	Bytes uint64
}

// ProcessStats represents process statistics - complete implementation
type ProcessStats struct {
	WaitOnResponse Timer

	// UBA_PROCESS_STATS fields (25 timers)
	Attach             Timer
	Detach             Timer
	Init               Timer
	CreateFile         Timer
	CloseFile          Timer
	GetFullFileName    Timer
	DeleteFile         Timer
	MoveFile           Timer
	Chmod              Timer
	CopyFile           Timer
	CreateProcess      Timer
	UpdateTables       Timer
	ListDirectory      Timer
	CreateTempFile     Timer
	OpenTempFile       Timer
	VirtualAllocFailed Timer
	Log                Timer
	SendFiles          Timer
	WriteFiles         Timer
	QueryCache         Timer
	WaitDecompress     Timer
	PreparseObjFiles   Timer
	FileTable          Timer
	DirTable           Timer
	LongPathName       Timer

	// Fixed fields
	StartupTime   uint64
	ExitTime      uint64
	WallTime      uint64
	CPUTime       uint64
	UsedMemory    uint32
	DetoursMemory uint64
	PeakMemory    uint64

	IopsRead      uint64
	IopsWrite     uint64
	IopsOther     uint64
	HostTotalTime uint64
}

// SessionStats represents session statistics - complete implementation
type SessionStats struct {
	// UBA_SESSION_STATS fields (13 timers)
	GetFileMsg         Timer
	GetBinaryMsg       Timer
	SendFileMsg        Timer
	ListDirMsg         Timer
	GetDirsMsg         Timer
	GetHashesMsg       Timer
	DeleteFileMsg      Timer
	CopyFileMsg        Timer
	CreateDirMsg       Timer
	WaitGetFileMsg     Timer
	CreateMmapFromFile Timer
	WaitMmapFromFile   Timer
	GetLongNameMsg     Timer
}

// StorageStats represents storage statistics - complete implementation
type StorageStats struct {
	// UBA_STORAGE_STATS fields (18 fields: 12 timers + 6 AtomicU64)
	CalculateCasKey    Timer
	CopyOrLink         Timer
	CopyOrLinkWait     Timer
	EnsureCas          Timer
	SendCas            Timer
	RecvCas            Timer
	CompressWrite      Timer
	CompressSend       Timer
	DecompressRecv     Timer
	DecompressToMem    Timer
	MemoryCopy         Timer
	HandleOverflow     Timer
	SendCasBytesRaw    uint64
	SendCasBytesComp   uint64
	RecvCasBytesRaw    uint64
	RecvCasBytesComp   uint64
	CreateCas          Timer
	CreateCasBytesRaw  uint64
	CreateCasBytesComp uint64
}

// ExtendedTimer represents an extended timer with bytes
type ExtendedTimer struct {
	Time  uint64
	Count uint32
	Bytes uint64
}

// KernelStats represents kernel statistics - complete implementation
type KernelStats struct {
	// UBA_KERNEL_STATS fields (14 fields) - correct types based on C++ definition
	CreateFile        ExtendedTimer // ExtendedTimer
	CloseFile         ExtendedTimer // ExtendedTimer
	WriteFile         TimeAndBytes  // TimeAndBytes
	MemoryCopy        TimeAndBytes  // TimeAndBytes
	ReadFile          TimeAndBytes  // TimeAndBytes
	SetFileInfo       ExtendedTimer // ExtendedTimer
	GetFileInfo       ExtendedTimer // ExtendedTimer
	CreateFileMapping ExtendedTimer // ExtendedTimer
	MapViewOfFile     ExtendedTimer // ExtendedTimer
	UnmapViewOfFile   ExtendedTimer // ExtendedTimer
	GetFileTime       ExtendedTimer // ExtendedTimer
	CloseHandle       ExtendedTimer // ExtendedTimer
	TraverseDir       ExtendedTimer // ExtendedTimer
	VirtualAlloc      ExtendedTimer // ExtendedTimer
}

// CacheStats represents cache statistics - complete implementation
type CacheStats struct {
	// UBA_CACHE_STATS fields (7 fields: 5 timers + 2 AtomicU64)
	FetchEntries   Timer
	FetchCasTable  Timer
	NormalizeFile  Timer
	TestEntry      Timer
	FetchOutput    Timer
	FetchBytesRaw  uint64
	FetchBytesComp uint64
}

// TraceView represents the main trace view structure
type TraceView struct {
	Sessions                []Session
	WorkTracks              []WorkTrack
	Strings                 []string
	StatusMap               map[uint64]StatusUpdate
	CacheWrites             map[uint32]CacheWrite
	RealStartTime           uint64
	StartTime               uint64
	SystemStartTimeUs       uint64
	Frequency               uint64
	TotalProcessActiveCount uint32
	TotalProcessExitedCount uint32
	ActiveSessionCount      uint32
	Version                 uint32
	ProgressProcessesTotal  uint32
	ProgressProcessesDone   uint32
	ProgressErrorCount      uint32
	RemoteExecutionDisabled bool
	Finished                bool
}

// ProcessLocation represents the location of a process
type ProcessLocation struct {
	SessionIndex   uint32
	ProcessorIndex uint32
	ProcessIndex   uint32
}

// WorkRecordLocation represents the location of a work record
type WorkRecordLocation struct {
	Track uint32
	Index uint32
}

// BinaryReader provides binary reading functionality
type BinaryReader struct {
	data []byte
	pos  int
}

// NewBinaryReader creates a new binary reader
func NewBinaryReader(data []byte) *BinaryReader {
	return &BinaryReader{data: data, pos: 0}
}

// GetPosition returns current position
func (r *BinaryReader) GetPosition() uint64 {
	return uint64(r.pos)
}

// SetPosition sets the current position
func (r *BinaryReader) SetPosition(pos uint64) {
	r.pos = int(pos)
}

// GetPositionData returns pointer to current position data
func (r *BinaryReader) GetPositionData() []byte {
	return r.data[r.pos:]
}

// Skip skips n bytes forward
func (r *BinaryReader) Skip(n uint64) {
	r.pos += int(n)
	if r.pos > len(r.data) {
		r.pos = len(r.data)
	}
}

// ReadByte reads a single byte
func (r *BinaryReader) ReadByte() uint8 {
	if r.pos >= len(r.data) {
		return 0
	}
	val := r.data[r.pos]
	r.pos++
	return val
}

// ReadU16 reads a 16-bit unsigned integer
func (r *BinaryReader) ReadU16() uint16 {
	if r.pos+2 > len(r.data) {
		return 0
	}
	val := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return val
}

// ReadU32 reads a 32-bit unsigned integer
func (r *BinaryReader) ReadU32() uint32 {
	if r.pos+4 > len(r.data) {
		return 0
	}
	val := binary.LittleEndian.Uint32(r.data[r.pos:])
	// val := binary.BigEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return val
}

// ReadU64 reads a 64-bit unsigned integer
func (r *BinaryReader) ReadU64() uint64 {
	if r.pos+8 > len(r.data) {
		return 0
	}
	val := binary.LittleEndian.Uint64(r.data[r.pos:])
	r.pos += 8
	return val
}

// ReadBool reads a boolean value
func (r *BinaryReader) ReadBool() bool {
	return r.ReadByte() != 0
}

// Read7BitEncoded reads a 7-bit encoded integer
func (r *BinaryReader) Read7BitEncoded() uint64 {
	var result uint64
	var shift uint
	startPos := r.pos
	for {
		if r.pos >= len(r.data) {
			break
		}
		b := r.data[r.pos]
		r.pos++
		result |= uint64(b&0x7F) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift > 63 {
			blog.Infof("ubatrace: Read7BitEncoded: Too many bytes for uint64 at position %d", startPos)
			break
		}
	}
	return result
}

// 获取调用位置信息
func callerInfo(skip int) string {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "unknown:0"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

// ReadString reads a string
func (r *BinaryReader) ReadString() (string, error) {
	length := r.Read7BitEncoded()

	// Check for unreasonably large string lengths (likely corrupted data)
	if length > 1000000 { // 1MB limit for strings
		blog.Infof("ubatrace: ReadString: caller(%s) Unreasonably large string length %d at position %d, likely corrupted data", callerInfo(1), length, r.pos)
		return "", fmt.Errorf("string with size %d is too large", length)
	}

	if r.pos+int(length) > len(r.data) {
		blog.Infof("ubatrace: ReadString: Trying to read %d bytes at position %d, but only %d bytes available", length, r.pos, len(r.data)-r.pos)
		return "", fmt.Errorf("string with size %d is too large", length)
	}
	str := string(r.data[r.pos : r.pos+int(length)])
	r.pos += int(length)
	return str, nil
}

func (r *BinaryReader) ReadLongString() (string, error) {
	// 读取第一个字节
	s := r.ReadByte()

	// 如果 s 为 0，直接读取普通字符串
	if s == 0 {
		return r.ReadString()
	}

	// 读取 7-bit 编码的字符串长度
	stringLength := r.Read7BitEncoded()

	uncompressedSize := stringLength * uint64(s)
	compressedSize := r.Read7BitEncoded()

	// 获取当前位置数据
	data := r.GetPositionData()

	// 跳过压缩数据
	r.Skip(uint64(compressedSize))

	// 根据 s 的值决定如何处理
	if s == uint8(unsafe.Sizeof(rune(0))) { // 假设 TString 是宽字符
		// 解压缩宽字符字符串 - 临时实现，直接返回空字符串
		_ = uncompressedSize // 避免未使用变量警告
		_ = data             // 避免未使用变量警告
		// TODO: 实现真正的解压缩算法
		blog.Infof("ubatrace: ReadLongString: Wide character string decompression not implemented yet, length=%d", stringLength)
		return "", nil
	} else {
		// 解压缩单字节字符串
		if s != 1 {
			return "", nil
		}

		_ = uncompressedSize // 避免未使用变量警告
		_ = data             // 避免未使用变量警告
		// TODO: 实现真正的解压缩算法
		blog.Infof("ubatrace: ReadLongString: Single byte string decompression not implemented yet, length=%d", stringLength)
		return "", nil
	}
}

// ReadGUID reads a GUID (16 bytes total)
func (r *BinaryReader) ReadGUID() GUID {
	var guid GUID
	if r.pos+16 > len(r.data) {
		blog.Infof("ubatrace: ReadGUID: Not enough data for GUID at position %d", r.pos)
		return guid
	}

	guid.Data1 = binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	guid.Data2 = binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	guid.Data3 = binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	copy(guid.Data4[:], r.data[r.pos:r.pos+8])
	r.pos += 8

	return guid
}

// ReadCasKey reads a CAS key
func (r *BinaryReader) ReadCasKey() CasKey {
	var key CasKey
	for i := 0; i < 20; i++ {
		key[i] = r.ReadByte()
	}
	return key
}

// ReadTimer reads a Timer structure
func (r *BinaryReader) ReadTimer() Timer {
	return Timer{
		Time:  r.Read7BitEncoded(),
		Count: uint32(r.Read7BitEncoded()),
	}
}

// ReadTimeAndBytes reads a TimeAndBytes structure
func (r *BinaryReader) ReadTimeAndBytes(version uint32) TimeAndBytes {
	timer := TimeAndBytes{
		Time:  r.Read7BitEncoded(),
		Count: uint32(r.Read7BitEncoded()),
	}
	if version >= 30 {
		timer.Bytes = r.Read7BitEncoded()
	}
	return timer
}

// ReadExtendedTimer reads an ExtendedTimer structure
// Note: ExtendedTimer in C++ only serializes time and count, not the longest field
func (r *BinaryReader) ReadExtendedTimer(version uint32) ExtendedTimer {
	return ExtendedTimer{
		Time:  r.Read7BitEncoded(),
		Count: uint32(r.Read7BitEncoded()),
		Bytes: 0, // longest field is not serialized
	}
}

// ReadProcessStats reads ProcessStats structure - complete implementation
func (r *BinaryReader) ReadProcessStats(version uint32) ProcessStats {
	stats := ProcessStats{}

	// Read waitOnResponse timer first
	stats.WaitOnResponse = r.ReadTimer()

	// For version < 30, read all stats individually based on version
	// For version >= 30, read based on bits
	if version < 30 {
		// Read all process stats based on version requirements
		// UBA_PROCESS_STAT(attach, 0) - version 0
		if version >= 0 {
			stats.Attach = r.ReadTimer()
		}
		if version >= 0 {
			stats.Detach = r.ReadTimer()
		}
		if version >= 0 {
			stats.Init = r.ReadTimer()
		}
		if version >= 0 {
			stats.CreateFile = r.ReadTimer()
		}
		if version >= 0 {
			stats.CloseFile = r.ReadTimer()
		}
		if version >= 0 {
			stats.GetFullFileName = r.ReadTimer()
		}
		if version >= 0 {
			stats.DeleteFile = r.ReadTimer()
		}
		if version >= 0 {
			stats.MoveFile = r.ReadTimer()
		}
		if version >= 17 {
			stats.Chmod = r.ReadTimer()
		}
		if version >= 0 {
			stats.CopyFile = r.ReadTimer()
		}
		if version >= 0 {
			stats.CreateProcess = r.ReadTimer()
		}
		if version >= 0 {
			stats.UpdateTables = r.ReadTimer()
		}
		if version >= 0 {
			stats.ListDirectory = r.ReadTimer()
		}
		if version >= 0 {
			stats.CreateTempFile = r.ReadTimer()
		}
		if version >= 0 {
			stats.OpenTempFile = r.ReadTimer()
		}
		if version >= 0 {
			stats.VirtualAllocFailed = r.ReadTimer()
		}
		if version >= 0 {
			stats.Log = r.ReadTimer()
		}
		if version >= 0 {
			stats.SendFiles = r.ReadTimer()
		}
		if version >= 19 {
			stats.WriteFiles = r.ReadTimer()
		}
		if version >= 24 {
			stats.QueryCache = r.ReadTimer()
		}
		if version >= 30 {
			stats.WaitDecompress = r.ReadTimer()
		}
		if version >= 30 {
			stats.PreparseObjFiles = r.ReadTimer()
		}
		if version >= 30 {
			stats.FileTable = r.ReadTimer()
		}
		if version >= 30 {
			stats.DirTable = r.ReadTimer()
		}
		if version >= 31 {
			stats.LongPathName = r.ReadTimer()
		}
	} else {
		bits := r.Read7BitEncoded()
		// Read stats based on bits (25 bits for 25 timers)
		if (bits & (1 << 0)) != 0 {
			stats.Attach = r.ReadTimer()
		}
		if (bits & (1 << 1)) != 0 {
			stats.Detach = r.ReadTimer()
		}
		if (bits & (1 << 2)) != 0 {
			stats.Init = r.ReadTimer()
		}
		if (bits & (1 << 3)) != 0 {
			stats.CreateFile = r.ReadTimer()
		}
		if (bits & (1 << 4)) != 0 {
			stats.CloseFile = r.ReadTimer()
		}
		if (bits & (1 << 5)) != 0 {
			stats.GetFullFileName = r.ReadTimer()
		}
		if (bits & (1 << 6)) != 0 {
			stats.DeleteFile = r.ReadTimer()
		}
		if (bits & (1 << 7)) != 0 {
			stats.MoveFile = r.ReadTimer()
		}
		if (bits & (1 << 8)) != 0 {
			stats.Chmod = r.ReadTimer()
		}
		if (bits & (1 << 9)) != 0 {
			stats.CopyFile = r.ReadTimer()
		}
		if (bits & (1 << 10)) != 0 {
			stats.CreateProcess = r.ReadTimer()
		}
		if (bits & (1 << 11)) != 0 {
			stats.UpdateTables = r.ReadTimer()
		}
		if (bits & (1 << 12)) != 0 {
			stats.ListDirectory = r.ReadTimer()
		}
		if (bits & (1 << 13)) != 0 {
			stats.CreateTempFile = r.ReadTimer()
		}
		if (bits & (1 << 14)) != 0 {
			stats.OpenTempFile = r.ReadTimer()
		}
		if (bits & (1 << 15)) != 0 {
			stats.VirtualAllocFailed = r.ReadTimer()
		}
		if (bits & (1 << 16)) != 0 {
			stats.Log = r.ReadTimer()
		}
		if (bits & (1 << 17)) != 0 {
			stats.SendFiles = r.ReadTimer()
		}
		if (bits & (1 << 18)) != 0 {
			stats.WriteFiles = r.ReadTimer()
		}
		if (bits & (1 << 19)) != 0 {
			stats.QueryCache = r.ReadTimer()
		}
		if (bits & (1 << 20)) != 0 {
			stats.WaitDecompress = r.ReadTimer()
		}
		if (bits & (1 << 21)) != 0 {
			stats.PreparseObjFiles = r.ReadTimer()
		}
		if (bits & (1 << 22)) != 0 {
			stats.FileTable = r.ReadTimer()
		}
		if (bits & (1 << 23)) != 0 {
			stats.DirTable = r.ReadTimer()
		}
		if (bits & (1 << 24)) != 0 {
			stats.LongPathName = r.ReadTimer()
		}
	}

	// Read the fixed fields at the end
	if version >= 37 {
		stats.StartupTime = r.Read7BitEncoded()
		stats.ExitTime = r.Read7BitEncoded()
		stats.WallTime = r.Read7BitEncoded()
		stats.CPUTime = r.Read7BitEncoded()
		stats.DetoursMemory = r.Read7BitEncoded()
		stats.PeakMemory = r.Read7BitEncoded()
		if version >= 39 {
			stats.IopsRead = r.Read7BitEncoded()
			stats.IopsWrite = r.Read7BitEncoded()
			stats.IopsOther = r.Read7BitEncoded()
		}
		stats.HostTotalTime = r.Read7BitEncoded()
	} else {
		stats.StartupTime = r.ReadU64()
		stats.ExitTime = r.ReadU64()
		stats.WallTime = r.ReadU64()
		stats.CPUTime = r.ReadU64()
		stats.DetoursMemory = uint64(r.ReadU32())
		stats.HostTotalTime = r.ReadU64()
	}
	return stats
}

// ReadSessionStats reads SessionStats structure - complete implementation
func (r *BinaryReader) ReadSessionStats(version uint32) SessionStats {
	stats := SessionStats{}

	if version < 30 {
		// Read all session stats based on version requirements
		if version >= 0 {
			stats.GetFileMsg = r.ReadTimer()
		}
		if version >= 0 {
			stats.GetBinaryMsg = r.ReadTimer()
		}
		if version >= 0 {
			stats.SendFileMsg = r.ReadTimer()
		}
		if version >= 0 {
			stats.ListDirMsg = r.ReadTimer()
		}
		if version >= 0 {
			stats.GetDirsMsg = r.ReadTimer()
		}
		if version >= 8 {
			stats.GetHashesMsg = r.ReadTimer()
		}
		if version >= 0 {
			stats.DeleteFileMsg = r.ReadTimer()
		}
		if version >= 16 {
			stats.CopyFileMsg = r.ReadTimer()
		}
		if version >= 0 {
			stats.CreateDirMsg = r.ReadTimer()
		}
		if version >= 10 {
			stats.WaitGetFileMsg = r.ReadTimer()
		}
		if version >= 12 {
			stats.CreateMmapFromFile = r.ReadTimer()
		}
		if version >= 12 {
			stats.WaitMmapFromFile = r.ReadTimer()
		}
		if version >= 31 {
			stats.GetLongNameMsg = r.ReadTimer()
		}
	} else {
		bits := r.ReadU16() // SessionStats uses u16 bits
		// Read stats based on bits (13 bits for 13 timers)
		if (bits & (1 << 0)) != 0 {
			stats.GetFileMsg = r.ReadTimer()
		}
		if (bits & (1 << 1)) != 0 {
			stats.GetBinaryMsg = r.ReadTimer()
		}
		if (bits & (1 << 2)) != 0 {
			stats.SendFileMsg = r.ReadTimer()
		}
		if (bits & (1 << 3)) != 0 {
			stats.ListDirMsg = r.ReadTimer()
		}
		if (bits & (1 << 4)) != 0 {
			stats.GetDirsMsg = r.ReadTimer()
		}
		if (bits & (1 << 5)) != 0 {
			stats.GetHashesMsg = r.ReadTimer()
		}
		if (bits & (1 << 6)) != 0 {
			stats.DeleteFileMsg = r.ReadTimer()
		}
		if (bits & (1 << 7)) != 0 {
			stats.CopyFileMsg = r.ReadTimer()
		}
		if (bits & (1 << 8)) != 0 {
			stats.CreateDirMsg = r.ReadTimer()
		}
		if (bits & (1 << 9)) != 0 {
			stats.WaitGetFileMsg = r.ReadTimer()
		}
		if (bits & (1 << 10)) != 0 {
			stats.CreateMmapFromFile = r.ReadTimer()
		}
		if (bits & (1 << 11)) != 0 {
			stats.WaitMmapFromFile = r.ReadTimer()
		}
		if (bits & (1 << 12)) != 0 {
			stats.GetLongNameMsg = r.ReadTimer()
		}
	}

	return stats
}

// ReadStorageStats reads StorageStats structure - complete implementation
func (r *BinaryReader) ReadStorageStats(version uint32) StorageStats {
	stats := StorageStats{}

	if version < 30 {
		// Read all storage stats based on version requirements
		if version >= 0 {
			stats.CalculateCasKey = r.ReadTimer()
		}
		if version >= 0 {
			stats.CopyOrLink = r.ReadTimer()
		}
		if version >= 0 {
			stats.CopyOrLinkWait = r.ReadTimer()
		}
		if version >= 0 {
			stats.EnsureCas = r.ReadTimer()
		}
		if version >= 0 {
			stats.SendCas = r.ReadTimer()
		}
		if version >= 0 {
			stats.RecvCas = r.ReadTimer()
		}
		if version >= 0 {
			stats.CompressWrite = r.ReadTimer()
		}
		if version >= 0 {
			stats.CompressSend = r.ReadTimer()
		}
		if version >= 0 {
			stats.DecompressRecv = r.ReadTimer()
		}
		if version >= 0 {
			stats.DecompressToMem = r.ReadTimer()
		}
		if version >= 30 {
			stats.MemoryCopy = r.ReadTimer()
		}
		if version >= 0 {
			stats.HandleOverflow = r.ReadTimer()
		}
		if version >= 0 {
			stats.SendCasBytesRaw = r.Read7BitEncoded()
		}
		if version >= 0 {
			stats.SendCasBytesComp = r.Read7BitEncoded()
		}
		if version >= 0 {
			stats.RecvCasBytesRaw = r.Read7BitEncoded()
		}
		if version >= 0 {
			stats.RecvCasBytesComp = r.Read7BitEncoded()
		}
		if version >= 0 {
			stats.CreateCas = r.ReadTimer()
		}
		if version >= 0 {
			stats.CreateCasBytesRaw = r.Read7BitEncoded()
		}
		if version >= 0 {
			stats.CreateCasBytesComp = r.Read7BitEncoded()
		}
	} else {
		bits := r.Read7BitEncoded()
		// Read stats based on bits (19 bits for 19 fields)
		if (bits & (1 << 0)) != 0 {
			stats.CalculateCasKey = r.ReadTimer()
		}
		if (bits & (1 << 1)) != 0 {
			stats.CopyOrLink = r.ReadTimer()
		}
		if (bits & (1 << 2)) != 0 {
			stats.CopyOrLinkWait = r.ReadTimer()
		}
		if (bits & (1 << 3)) != 0 {
			stats.EnsureCas = r.ReadTimer()
		}
		if (bits & (1 << 4)) != 0 {
			stats.SendCas = r.ReadTimer()
		}
		if (bits & (1 << 5)) != 0 {
			stats.RecvCas = r.ReadTimer()
		}
		if (bits & (1 << 6)) != 0 {
			stats.CompressWrite = r.ReadTimer()
		}
		if (bits & (1 << 7)) != 0 {
			stats.CompressSend = r.ReadTimer()
		}
		if (bits & (1 << 8)) != 0 {
			stats.DecompressRecv = r.ReadTimer()
		}
		if (bits & (1 << 9)) != 0 {
			stats.DecompressToMem = r.ReadTimer()
		}
		if (bits & (1 << 10)) != 0 {
			stats.MemoryCopy = r.ReadTimer()
		}
		if (bits & (1 << 11)) != 0 {
			stats.HandleOverflow = r.ReadTimer()
		}
		if (bits & (1 << 12)) != 0 {
			stats.SendCasBytesRaw = r.Read7BitEncoded()
		}
		if (bits & (1 << 13)) != 0 {
			stats.SendCasBytesComp = r.Read7BitEncoded()
		}
		if (bits & (1 << 14)) != 0 {
			stats.RecvCasBytesRaw = r.Read7BitEncoded()
		}
		if (bits & (1 << 15)) != 0 {
			stats.RecvCasBytesComp = r.Read7BitEncoded()
		}
		if (bits & (1 << 16)) != 0 {
			stats.CreateCas = r.ReadTimer()
		}
		if (bits & (1 << 17)) != 0 {
			stats.CreateCasBytesRaw = r.Read7BitEncoded()
		}
		if (bits & (1 << 18)) != 0 {
			stats.CreateCasBytesComp = r.Read7BitEncoded()
		}
	}

	return stats
}

// ReadKernelStats reads KernelStats structure - complete implementation
func (r *BinaryReader) ReadKernelStats(version uint32) KernelStats {
	stats := KernelStats{}

	if version < 30 {
		// Read all kernel stats based on version requirements
		if version >= 0 {
			stats.CreateFile = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.CloseFile = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.WriteFile = r.ReadTimeAndBytes(version) // TimeAndBytes
		}
		if version >= 30 {
			stats.MemoryCopy = r.ReadTimeAndBytes(version) // TimeAndBytes
		}
		if version >= 0 {
			stats.ReadFile = r.ReadTimeAndBytes(version) // TimeAndBytes
		}
		if version >= 0 {
			stats.SetFileInfo = r.ReadExtendedTimer(version)
		}
		if version >= 29 {
			stats.GetFileInfo = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.CreateFileMapping = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.MapViewOfFile = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.UnmapViewOfFile = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.GetFileTime = r.ReadExtendedTimer(version)
		}
		if version >= 0 {
			stats.CloseHandle = r.ReadExtendedTimer(version)
		}
		if version >= 27 {
			stats.TraverseDir = r.ReadExtendedTimer(version)
		}
		if version >= 30 {
			stats.VirtualAlloc = r.ReadExtendedTimer(version)
		}
	} else {
		var bits uint64
		// Read stats based on bits (14 bits for 14 fields)
		if version < 43 {
			bits = uint64(r.ReadU16())
		} else {
			bits = r.Read7BitEncoded()
		}
		if (bits & (1 << 0)) != 0 {
			stats.CreateFile = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 1)) != 0 {
			stats.CloseFile = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 2)) != 0 {
			stats.WriteFile = r.ReadTimeAndBytes(version) // TimeAndBytes
		}
		if (bits & (1 << 3)) != 0 {
			stats.MemoryCopy = r.ReadTimeAndBytes(version) // TimeAndBytes
		}
		if (bits & (1 << 4)) != 0 {
			stats.ReadFile = r.ReadTimeAndBytes(version) // TimeAndBytes
		}
		if (bits & (1 << 5)) != 0 {
			stats.SetFileInfo = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 6)) != 0 {
			stats.GetFileInfo = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 7)) != 0 {
			stats.CreateFileMapping = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 8)) != 0 {
			stats.MapViewOfFile = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 9)) != 0 {
			stats.UnmapViewOfFile = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 10)) != 0 {
			stats.GetFileTime = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 11)) != 0 {
			stats.CloseHandle = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 12)) != 0 {
			stats.TraverseDir = r.ReadExtendedTimer(version)
		}
		if (bits & (1 << 13)) != 0 {
			stats.VirtualAlloc = r.ReadExtendedTimer(version)
		}
	}

	return stats
}

// ReadCacheStats reads CacheStats structure - complete implementation
func (r *BinaryReader) ReadCacheStats(version uint32) CacheStats {
	stats := CacheStats{}

	// CacheStats doesn't use version-based reading like others
	// It reads all fields directly
	stats.FetchEntries = r.ReadTimer()
	stats.FetchCasTable = r.ReadTimer()
	if version >= 28 {
		stats.NormalizeFile = r.ReadTimer()
	}
	stats.TestEntry = r.ReadTimer()
	stats.FetchOutput = r.ReadTimer()
	if version >= 26 {
		stats.FetchBytesRaw = r.Read7BitEncoded()
	}
	if version >= 26 {
		stats.FetchBytesComp = r.Read7BitEncoded()
	}

	return stats
}

// TraceReader handles trace reading operations
type TraceReader struct {
	activeProcesses       map[uint32]ProcessLocation
	activeWorkRecords     map[uint32]WorkRecordLocation
	sessionIndexToSession []uint32
}

// NewTraceReader creates a new trace reader
func NewTraceReader() *TraceReader {
	return &TraceReader{
		activeProcesses:       make(map[uint32]ProcessLocation),
		activeWorkRecords:     make(map[uint32]WorkRecordLocation),
		sessionIndexToSession: make([]uint32, 0),
	}
}

// ConvertTime converts time using frequency
func ConvertTime(view *TraceView, time uint64) uint64 {
	// Assuming GetFrequency() returns a standard frequency
	// This would need to be implemented based on the actual system
	frequency := uint64(10000000) // 10MHz as example
	return time * frequency / view.Frequency
}

// GetFrequency returns the system frequency
func GetFrequency() uint64 {
	return uint64(10000000) // 10MHz as example - would need actual implementation
}

// TimeToS converts time to seconds
func TimeToS(time uint64) float32 {
	return float32(time) / float32(GetFrequency())
}

// Max returns the maximum of two float32 values
func Max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// ReadClientId reads a client ID from the reader
func (tr *TraceReader) ReadClientId(out *TraceView, reader *BinaryReader) GUID {
	if out.Version < 15 {
		return reader.ReadGUID()
	}

	var guid GUID
	guid.Data1 = uint32(reader.Read7BitEncoded())
	return guid
}

// GetSession gets a session by index
func (tr *TraceReader) GetSession(out *TraceView, sessionIndex uint32) *Session {
	if int(sessionIndex) >= len(tr.sessionIndexToSession) {
		return nil
	}
	virtualIndex := tr.sessionIndexToSession[sessionIndex]
	if int(virtualIndex) >= len(out.Sessions) {
		return nil
	}
	return &out.Sessions[virtualIndex]
}

// GetSessionByUID gets a session by client UID
func (tr *TraceReader) GetSessionByUID(out *TraceView, clientUID GUID) *Session {
	for i := range out.Sessions {
		if out.Sessions[i].ClientUID == clientUID {
			return &out.Sessions[i]
		}
	}
	return nil
}

// ProcessBegin starts a new process
func (tr *TraceReader) ProcessBegin(out *TraceView, sessionIndex, id uint32, time uint64, description string, breadcrumbs string) *Process {
	session := tr.GetSession(out, sessionIndex)
	if session == nil {
		return nil
	}

	if len(session.Processors) == 0 {
		session.Processors = append(session.Processors, Processor{})
	}

	processor := &session.Processors[0]
	processor.Processes = append(processor.Processes, Process{
		ID:          id,
		Description: description,
		Start:       time,
		Stop:        ^uint64(0), // Max uint64
		ExitCode:    ^uint32(0), // Max uint32
		Breadcrumbs:  breadcrumbs,
		IsRemote:    sessionIndex != 0,
	})

	processIndex := uint32(len(processor.Processes) - 1)
	tr.activeProcesses[id] = ProcessLocation{
		SessionIndex:   sessionIndex,
		ProcessorIndex: 0,
		ProcessIndex:   processIndex,
	}

	session.ProcessActiveCount++
	out.TotalProcessActiveCount++

	return &processor.Processes[processIndex]
}

// ProcessEnd ends a process
func (tr *TraceReader) ProcessEnd(out *TraceView, id uint32, time uint64) (*Process, uint32) {
	location, exists := tr.activeProcesses[id]
	if !exists {
		return nil, 0
	}

	delete(tr.activeProcesses, id)

	session := tr.GetSession(out, location.SessionIndex)
	if session == nil {
		return nil, location.SessionIndex
	}

	session.ProcessActiveCount--
	out.TotalProcessActiveCount--

	process := &session.Processors[location.ProcessorIndex].Processes[location.ProcessIndex]
	process.Stop = time
	process.BitmapDirty = true

	session.ProcessExitedCount++
	out.TotalProcessExitedCount++

	return process, location.SessionIndex
}

// StopAllActive stops all active processes
func (tr *TraceReader) StopAllActive(out *TraceView, stopTime uint64) {
	for _, location := range tr.activeProcesses {
		session := tr.GetSession(out, location.SessionIndex)
		if session != nil {
			process := &session.Processors[location.ProcessorIndex].Processes[location.ProcessIndex]
			process.ExitCode = ^uint32(0)
			process.Stop = stopTime
			process.BitmapDirty = true
		}
	}

	tr.activeProcesses = make(map[uint32]ProcessLocation)
	out.Finished = true
}

// ReadTrace reads and processes a trace entry - main function equivalent to C++ version
func (tr *TraceReader) ReadTrace(out *TraceView, reader *BinaryReader, maxTime uint64, traceTypeCounts map[uint8]int) bool {
	readPos := reader.GetPosition()

	// Check if we have enough data to read at least one byte
	if reader.pos >= len(reader.data) {
		return false
	}

	traceType := reader.ReadByte()
	var time uint64 = 0

	// Count trace types
	if traceTypeCounts != nil {
		traceTypeCounts[traceType]++
	}

	// Print the trace type for each message
	if _, exists := TraceTypeNames[traceType]; exists {
		if TraceTypeProgressUpdate != traceType {
		}
	} else {
		blog.Infof("ubatrace: Processing unknown trace type: %d", traceType)
		return false
	}

	if out.Version >= 15 && traceType != TraceTypeString && traceType != TraceTypeDriveUpdate {
		time = ConvertTime(out, reader.Read7BitEncoded())
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}
	
	switch traceType {
	case TraceTypeSessionAdded:
		return tr.handleSessionAdded(out, reader, time)
	case TraceTypeSessionUpdate:
		return tr.handleSessionUpdate(out, reader, time, maxTime, readPos)
	case TraceTypeSessionDisconnect:
		return tr.handleSessionDisconnect(out, reader, time, maxTime, readPos)
	case TraceTypeSessionNotification:
		return tr.handleSessionNotification(out, reader)
	case TraceTypeSessionSummary:
		return tr.handleSessionSummary(out, reader)
	case TraceTypeProcessAdded:
		return tr.handleProcessAdded(out, reader, time, maxTime, readPos)
	case TraceTypeProcessExited:
		return tr.handleProcessExited(out, reader, time, maxTime, readPos)
	case TraceTypeProcessEnvironmentUpdated:
		return tr.handleProcessEnvironmentUpdated(out, reader, time)
	case TraceTypeProcessReturned:
		return tr.handleProcessReturned(out, reader, time, maxTime, readPos)
	case TraceTypeFileBeginFetch:
		return tr.handleFileBeginFetch(out, reader, time, maxTime, readPos)
	case TraceTypeFileFetchLight:
		return tr.handleFileFetchLight(out, reader, time)
	case TraceTypeProxyCreated:
		return tr.handleProxyCreated(out, reader, time, maxTime, readPos)
	case TraceTypeProxyUsed:
		return tr.handleProxyUsed(out, reader, time, maxTime, readPos)
	case TraceTypeFileEndFetch:
		return tr.handleFileEndFetch(out, reader, time, maxTime, readPos)
	case TraceTypeFileBeginStore:
		return tr.handleFileBeginStore(out, reader, time, maxTime, readPos)
	case TraceTypeFileStoreLight:
		return tr.handleFileStoreLight(out, reader, time)
	case TraceTypeFileEndStore:
		return tr.handleFileEndStore(out, reader, time, maxTime, readPos)
	case TraceTypeSummary:
		return tr.handleSummary(out, reader, time)
	case TraceTypeBeginWork:
		return tr.handleBeginWork(out, reader, time)
	case TraceTypeEndWork:
		return tr.handleEndWork(out, reader, time)
	case TraceTypeProgressUpdate:
		return tr.handleProgressUpdate(out, reader)
	case TraceTypeStatusUpdate:
		return tr.handleStatusUpdate(out, reader)
	case TraceTypeRemoteExecutionDisabled:
		return tr.handleRemoteExecutionDisabled(out, reader)
	case TraceTypeString:
		return tr.handleString(out, reader)
	case TraceTypeCacheBeginFetch:
		return tr.handleCacheBeginFetch(out, reader, time)
	case TraceTypeCacheEndFetch:
		return tr.handleCacheEndFetch(out, reader, time)
	case TraceTypeCacheBeginWrite:
		return tr.handleCacheBeginWrite(out, reader, time)
	case TraceTypeCacheEndWrite:
		return tr.handleCacheEndWrite(out, reader, time)
	case TraceTypeFileFetchSize: 
		return tr.handleFileFetchSize(out, reader)
	case TraceTypeProcessBreadcrumbs:
		return tr.handleProcessBreadcrumbs(out, reader)
	case TraceTypeWorkHint:
		return tr.handleWorkHint(out, reader, time)
	case TraceTypeDriveUpdate:
		return tr.handleDriverUpdate(out, reader)
	}

	return true
}

// resize 通用切片调整大小函数，支持任意类型的切片
func resize[T any](arr []T, newSize int) []T {
	if newSize > len(arr) {
		// 一次性分配足够的容量，避免多次内存分配
		newArr := make([]T, newSize)
		copy(newArr, arr)
		// 剩余部分已经是零值，无需额外填充
		return newArr
	} else if newSize < len(arr) {
		return arr[:newSize]
	}
	return arr
}

// handleSessionAdded handles TraceType_SessionAdded
func (tr *TraceReader) handleSessionAdded(out *TraceView, reader *BinaryReader, time uint64) bool {
	sessionName, err := reader.ReadString()
	if err != nil {
		return false
	}
	sessionInfo, err := reader.ReadString()
	if err != nil {
		return false
	}
	clientUID := tr.ReadClientId(out, reader)
	sessionIndex := reader.ReadU32()

	fullName := fmt.Sprintf("%s (%s)", sessionName, sessionInfo)

	// Check if we can re-use existing session
	virtualSessionIndex := sessionIndex
	for i := range out.Sessions {
		oldSession := &out.Sessions[i]
		if oldSession.FullName != fullName {
			continue
		}
		if oldSession.DisconnectTime == ^uint64(0) {
			break
		}
		updateCount := len(oldSession.Updates)
		// 批量调整session统计数据的slice大小
		oldSession.NetworkSend, oldSession.NetworkRecv, oldSession.Ping, oldSession.MemAvail =
			resize(oldSession.NetworkSend, updateCount),
			resize(oldSession.NetworkRecv, updateCount),
			resize(oldSession.Ping, updateCount),
			resize(oldSession.MemAvail, updateCount)
		oldSession.CpuLoad = resize(oldSession.CpuLoad, updateCount)
		oldSession.ConnectionCount = resize(oldSession.ConnectionCount, updateCount)

		oldSession.ReconnectIndices = append(oldSession.ReconnectIndices, uint32(updateCount))
		oldSession.IsReset = true
		oldSession.DisconnectTime = ^uint64(0)
		oldSession.ProxyName = ""
		oldSession.ProxyCreated = false
		oldSession.Notification = ""
		virtualSessionIndex = uint32(i)
		break
	}

	// Check for reasonable session index values to prevent memory issues
	if sessionIndex > 10000 || virtualSessionIndex > 10000 {
		blog.Infof("ubatrace: Warning: Very large session index detected (sessionIndex: %d, virtualSessionIndex: %d), skipping", sessionIndex, virtualSessionIndex)
		return true
	}

	// Resize sessions if needed
	for len(out.Sessions) <= int(virtualSessionIndex) {
		out.Sessions = append(out.Sessions, Session{})
	}

	// Resize session index mapping if needed
	for len(tr.sessionIndexToSession) <= int(sessionIndex) {
		tr.sessionIndexToSession = append(tr.sessionIndexToSession, 0)
	}

	tr.sessionIndexToSession[sessionIndex] = virtualSessionIndex

	session := &out.Sessions[virtualSessionIndex]
	session.Name = sessionName
	session.FullName = fullName
	session.ClientUID = clientUID

	out.ActiveSessionCount++
	return true
}

// handleSessionUpdate handles TraceType_SessionUpdate
func (tr *TraceReader) handleSessionUpdate(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	var sessionIndex uint32
	var connectionCount uint8 = 0
	var totalSend, totalRecv, lastPing uint64

	if out.Version >= 14 {
		sessionIndex = uint32(reader.Read7BitEncoded())
		connectionCount = uint8(reader.Read7BitEncoded())
		totalSend = reader.Read7BitEncoded()
		totalRecv = reader.Read7BitEncoded()
		lastPing = reader.Read7BitEncoded()
	} else {
		sessionIndex = reader.ReadU32()
		totalSend = reader.ReadU64()
		totalRecv = reader.ReadU64()
		lastPing = reader.Read7BitEncoded()
	}

	var memAvail, memTotal uint64 = 0, 0
	if out.Version >= 9 {
		memAvail = reader.Read7BitEncoded()
		memTotal = reader.Read7BitEncoded()
	}

	var cpuLoad float32 = 0
	if out.Version >= 13 {
		value := reader.ReadU32()
		cpuLoad = math.Float32frombits(value)
	}

	session := tr.GetSession(out, sessionIndex)
	if session == nil {
		return true // Continue processing instead of stopping
	}

	if session.IsReset {
		session.IsReset = false
		session.PrevUpdateTime = 0
		session.PrevSend = 0
		session.PrevRecv = 0
		session.MemTotal = 0
		if len(session.Updates) > 0 {
			session.Updates = append(session.Updates, time)
			session.NetworkSend = append(session.NetworkSend, 0)
			session.NetworkRecv = append(session.NetworkRecv, 0)
			session.Ping = append(session.Ping, lastPing)
			session.MemAvail = append(session.MemAvail, memAvail)
			session.CpuLoad = append(session.CpuLoad, cpuLoad)
			session.ConnectionCount = append(session.ConnectionCount, uint16(connectionCount))
		}
	} else if len(session.Updates) > 0 {
		session.PrevSend = session.NetworkSend[len(session.NetworkSend)-1]
		session.PrevRecv = session.NetworkRecv[len(session.NetworkRecv)-1]
		session.PrevUpdateTime = session.Updates[len(session.Updates)-1]
	}

	if totalSend < session.PrevSend {
		totalSend = session.PrevSend
	}
	if totalRecv < session.PrevRecv {
		totalRecv = session.PrevRecv
	}

	session.MemTotal = memTotal
	session.Updates = append(session.Updates, time)
	session.NetworkSend = append(session.NetworkSend, totalSend)
	session.NetworkRecv = append(session.NetworkRecv, totalRecv)
	session.Ping = append(session.Ping, lastPing)
	session.MemAvail = append(session.MemAvail, memAvail)
	session.CpuLoad = append(session.CpuLoad, cpuLoad)
	session.ConnectionCount = append(session.ConnectionCount, uint16(connectionCount))

	if session.PrevUpdateTime > 0 {
		session.HighestSendPerS = Max(session.HighestSendPerS, float32(totalSend-session.PrevSend)/TimeToS(time-session.PrevUpdateTime))
		session.HighestRecvPerS = Max(session.HighestRecvPerS, float32(totalRecv-session.PrevRecv)/TimeToS(time-session.PrevUpdateTime))
	}

	return true
}

// handleSessionDisconnect handles TraceType_SessionDisconnect
func (tr *TraceReader) handleSessionDisconnect(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	sessionIndex := reader.ReadU32()
	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSession(out, sessionIndex)
	if session == nil {
		blog.Infof("ubatrace: Warning: SessionDisconnect for sessionIndex %d but session not found, continuing...", sessionIndex)
		return true
	}

	session.DisconnectTime = time
	session.MaxVisibleFiles = 0

	// Stop all fetched files
	for i := range session.FetchedFiles {
		if session.FetchedFiles[i].Stop == ^uint64(0) {
			session.FetchedFiles[i].Stop = time
		}
	}

	// Stop all stored files
	for i := range session.StoredFiles {
		if session.StoredFiles[i].Stop == ^uint64(0) {
			session.StoredFiles[i].Stop = time
		}
	}

	out.ActiveSessionCount--
	return true
}

// handleSessionNotification handles TraceType_SessionNotification
func (tr *TraceReader) handleSessionNotification(out *TraceView, reader *BinaryReader) bool {
	sessionIndex := reader.ReadU32()
	session := tr.GetSession(out, sessionIndex)
	if session == nil {
		blog.Infof("ubatrace: Warning: SessionNotification for sessionIndex %d but session not found, continuing...", sessionIndex)
		return true
	}
	var err error
	session.Notification, err = reader.ReadString()
	if err != nil {
		return false
	}
	return true
}

// handleSessionSummary handles TraceType_SessionSummary
func (tr *TraceReader) handleSessionSummary(out *TraceView, reader *BinaryReader) bool {
	sessionIndex := reader.ReadU32()
	lineCount := reader.ReadU32()

	session := tr.GetSession(out, sessionIndex)
	if session == nil {
		blog.Infof("ubatrace: Warning: SessionSummary for sessionIndex %d but session not found, continuing...", sessionIndex)
		return true
	}

	session.Summary = make([]string, 0, lineCount)
	for i := uint32(0); i < lineCount; i++ {
		line, err := reader.ReadString()
		if err != nil {
			return false
		}
		session.Summary = append(session.Summary, line)
	}
	return true
}

// handleProcessAdded handles TraceType_ProcessAdded
func (tr *TraceReader) handleProcessAdded(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	sessionIndex := reader.ReadU32()
	id := reader.ReadU32()
	desc, err := reader.ReadString()

	var breadcrumbs string
	if out.Version >= 35 {
		if out.Version < 38 {
			breadcrumbs, _ = reader.ReadString()
		} else if out.Version < 42 {
			if reader.ReadBool() {
				breadcrumbs, _ = reader.ReadString()
			} else {
				reader.Read7BitEncoded()
				reader.Skip(reader.Read7BitEncoded())
				//reader.Skip(reader.Read7BitEncoded());
				breadcrumbs = "Upgrade your visualizer"
			}
		} else {
			breadcrumbs, _ = reader.ReadLongString()
		}
	}
	if err != nil {
		return false
	}

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	tr.ProcessBegin(out, sessionIndex, id, time, desc, breadcrumbs)
	return true
}

// handleProcessExited handles TraceType_ProcessExited
func (tr *TraceReader) handleProcessExited(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	id := reader.ReadU32()
	exitCode := reader.ReadU32()
	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	process, _ := tr.ProcessEnd(out, id, time)
	if process == nil {
		blog.Infof("ubatrace: Warning: ProcessExited for processId %d but process not found, continuing...", id)
		return true
	}

	process.ExitCode = exitCode

	// Create stats objects - exactly like C++
	var processStats ProcessStats
	var sessionStats SessionStats
	var storageStats StorageStats
	var kernelStats KernelStats

	// Read stats data - exactly like C++
	dataStart := reader.GetPosition()
	processStats = reader.ReadProcessStats(out.Version)

	// Read additional stats based on process type and version - exactly like C++
	if process.IsRemote { // equivalent to process.isRemote
		if out.Version >= 7 {
			sessionStats = reader.ReadSessionStats(out.Version)
			storageStats = reader.ReadStorageStats(out.Version)
			kernelStats = reader.ReadKernelStats(out.Version)
		}
	} else if out.Version >= 30 {
		if out.Version >= 36 {
			sessionStats = reader.ReadSessionStats(out.Version)
		}
		storageStats = reader.ReadStorageStats(out.Version)
		kernelStats = reader.ReadKernelStats(out.Version)
	}

	// Copy stats data - exactly like C++
	dataEnd := reader.GetPosition()
	statsSize := dataEnd - dataStart
	process.Stats = make([]byte, statsSize)
	if statsSize > 0 {
		copy(process.Stats, reader.data[dataStart:dataEnd])
	}

	var err error
	// Read breadcrumbs if version >= 34 - exactly like C++
	if out.Version >= 34 && out.Version < 35 {
		process.Breadcrumbs, err = reader.ReadString()
		if err != nil {
			return false
		}
	}

	// Set timing fields from processStats - exactly like C++
	process.CreateFilesTime = processStats.CreateFile.Time
	if processStats.WriteFiles.Time > processStats.SendFiles.Time {
		process.WriteFilesTime = processStats.WriteFiles.Time
	} else {
		process.WriteFilesTime = processStats.SendFiles.Time
	}

	// Avoid unused variable warnings
	_ = sessionStats
	_ = storageStats
	_ = kernelStats

	// Read log lines based on version - exactly like C++
	if out.Version >= 22 {
		for {
			logType := reader.ReadByte()
			if logType == 255 {
				break
			}
			logLine, err := reader.ReadString()
			if err != nil {
				return false
			}
			process.LogLines = append(process.LogLines, ProcessLogLine{
				Text: logLine,
				Type: logType,
			})
		}
	} else if out.Version >= 20 {
		logLineCount := reader.Read7BitEncoded()
		if logLineCount >= 101 {
			logLineCount = 101
		}
		process.LogLines = make([]ProcessLogLine, 0, logLineCount)
		for i := uint64(0); i < logLineCount; i++ {
			logType := reader.ReadByte()
			logLine, err := reader.ReadString()
			if err != nil {
				return false
			}
			process.LogLines = append(process.LogLines, ProcessLogLine{
				Text: logLine,
				Type: logType,
			})
		}
	}

	return true
}

// handleProcessEnvironmentUpdated handles TraceType_ProcessEnvironmentUpdated
func (tr *TraceReader) handleProcessEnvironmentUpdated(out *TraceView, reader *BinaryReader, time uint64) bool {
	processID := reader.ReadU32()
	location, exists := tr.activeProcesses[processID]
	if !exists {
		blog.Infof("ubatrace: Warning: ProcessEnvironmentUpdated for processId %d but process not found, continuing...", processID)
		return true
	}

	reason, err := reader.ReadString()
	if err != nil {
		return false
	}
	if out.Version < 15 {
		time = reader.Read7BitEncoded()
	}

	session := tr.GetSession(out, location.SessionIndex)
	if session == nil {
		blog.Infof("ubatrace: Warning: ProcessEnvironmentUpdated session for sessionIndex %d not found, continuing...", location.SessionIndex)
		return true
	}

	processes := &session.Processors[location.ProcessorIndex].Processes
	process := &(*processes)[location.ProcessIndex]

	// Read stats according to C++ implementation
	dataStart := reader.GetPosition()

	// Read ProcessStats, SessionStats, StorageStats, KernelStats
	processStats := reader.ReadProcessStats(out.Version)
	sessionStats := reader.ReadSessionStats(out.Version)
	storageStats := reader.ReadStorageStats(out.Version)
	kernelStats := reader.ReadKernelStats(out.Version)

	_ = processStats // Avoid unused variable warning
	_ = sessionStats // Avoid unused variable warning
	_ = storageStats // Avoid unused variable warning
	_ = kernelStats  // Avoid unused variable warning

	dataEnd := reader.GetPosition()
	statsSize := dataEnd - dataStart
	process.Stats = make([]byte, statsSize)
	// Copy the stats data from the original position
	if statsSize > 0 {
		copy(process.Stats, reader.data[dataStart:dataEnd])
	}

	process.ExitCode = 0
	process.Stop = time
	process.BitmapDirty = true
	isRemote := process.IsRemote

	// Create new process
	*processes = append(*processes, Process{
		ID:          processID,
		Description: reason,
		Start:       time,
		Stop:        ^uint64(0),
		ExitCode:    ^uint32(0),
		IsRemote:    isRemote,
	})

	tr.activeProcesses[processID] = ProcessLocation{
		SessionIndex:   location.SessionIndex,
		ProcessorIndex: location.ProcessorIndex,
		ProcessIndex:   uint32(len(*processes) - 1),
	}

	session.ProcessExitedCount++
	out.TotalProcessExitedCount++
	return true
}

// handleProcessReturned handles TraceType_ProcessReturned
func (tr *TraceReader) handleProcessReturned(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	id := reader.ReadU32()
	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	var reason string = "Unknown"
	var err error
	if out.Version >= 33 {
		reason, err = reader.ReadString()
		if err != nil {
			return false
		}
		if reason == "" {
			reason = "Unknown"
		}
	}

	location, exists := tr.activeProcesses[id]
	if !exists {
		blog.Infof("ubatrace: Warning: ProcessReturned for processId %d but process not found, continuing...", id)
		return true
	}
	delete(tr.activeProcesses, id)

	session := tr.GetSession(out, location.SessionIndex)
	if session == nil {
		blog.Infof("ubatrace: Warning: ProcessReturned session for sessionIndex %d not found, continuing...", location.SessionIndex)
		return true
	}

	session.ProcessActiveCount--
	out.TotalProcessActiveCount--

	process := &session.Processors[location.ProcessorIndex].Processes[location.ProcessIndex]
	process.ExitCode = 0
	process.Stop = time
	process.ReturnedReason = reason
	process.BitmapDirty = true

	return true
}

// handleFileBeginFetch handles TraceType_FileBeginFetch
func (tr *TraceReader) handleFileBeginFetch(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	key := reader.ReadCasKey()
	var size uint64
	size = 0
	if out.Version < 36 {
		size = reader.Read7BitEncoded()
	}

	var hint string
	var err error
	if out.Version < 14 {
		hint, err = reader.ReadString()
		if err != nil {
			return false
		}
	} else {
		stringIndex := reader.Read7BitEncoded()
		if int(stringIndex) < len(out.Strings) {
			hint = out.Strings[stringIndex]
		}
	}

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		session.FetchedFiles = append(session.FetchedFiles, FileTransfer{
			Key:   key,
			Size:  size,
			Hint:  hint,
			Start: time,
			Stop:  ^uint64(0),
		})
		session.FetchedFilesBytes += size
	}
	return true
}

// handleFileFetchLight handles TraceType_FileFetchLight
func (tr *TraceReader) handleFileFetchLight(out *TraceView, reader *BinaryReader, time uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	fileSize := reader.Read7BitEncoded()

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		session.FetchedFiles = append(session.FetchedFiles, FileTransfer{
			Key:   CasKeyZero,
			Size:  fileSize,
			Hint:  "",
			Start: time,
			Stop:  time,
		})
		session.FetchedFilesBytes += fileSize
	}
	return true
}

// handleProxyCreated handles TraceType_ProxyCreated
func (tr *TraceReader) handleProxyCreated(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	proxyName, err := reader.ReadString()
	if err != nil {
		return false
	}

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		session.ProxyName = proxyName
		session.ProxyCreated = true
	}
	return true
}

// handleProxyUsed handles TraceType_ProxyUsed
func (tr *TraceReader) handleProxyUsed(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	proxyName, err := reader.ReadString()
	if err != nil {
		return false
	}

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		session.ProxyName = proxyName
	}
	return true
}

// handleFileEndFetch handles TraceType_FileEndFetch
func (tr *TraceReader) handleFileEndFetch(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	key := reader.ReadCasKey()

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		// Find the matching fetch operation and mark it as complete
		for i := len(session.FetchedFiles) - 1; i >= 0; i-- {
			if session.FetchedFiles[i].Key == key {
				session.FetchedFiles[i].Stop = time
				break
			}
		}
	}
	return true
}

// handleFileBeginStore handles TraceType_FileBeginStore
func (tr *TraceReader) handleFileBeginStore(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	key := reader.ReadCasKey()
	size := reader.Read7BitEncoded()

	var hint string
	var err error
	if out.Version < 14 {
		hint, err = reader.ReadString()
		if err != nil {
			return false
		}
	} else {
		stringIndex := reader.Read7BitEncoded()
		if int(stringIndex) < len(out.Strings) {
			hint = out.Strings[stringIndex]
		}
	}

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		session.StoredFiles = append(session.StoredFiles, FileTransfer{
			Key:   key,
			Size:  size,
			Hint:  hint,
			Start: time,
			Stop:  ^uint64(0),
		})
		session.StoredFilesBytes += size
	}
	return true
}

// handleFileStoreLight handles TraceType_FileStoreLight
func (tr *TraceReader) handleFileStoreLight(out *TraceView, reader *BinaryReader, time uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	fileSize := reader.Read7BitEncoded()

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		session.StoredFiles = append(session.StoredFiles, FileTransfer{
			Key:   CasKeyZero,
			Size:  fileSize,
			Hint:  "",
			Start: time,
			Stop:  time,
		})
		session.StoredFilesBytes += fileSize
	}
	return true
}

// handleFileEndStore handles TraceType_FileEndStore
func (tr *TraceReader) handleFileEndStore(out *TraceView, reader *BinaryReader, time uint64, maxTime uint64, readPos uint64) bool {
	clientUID := tr.ReadClientId(out, reader)
	key := reader.ReadCasKey()

	if out.Version < 15 {
		time = reader.Read7BitEncoded()
		if time > maxTime {
			reader.SetPosition(readPos)
			return true
		}
	}

	session := tr.GetSessionByUID(out, clientUID)
	if session != nil {
		// Find the matching store operation and mark it as complete
		for i := len(session.StoredFiles) - 1; i >= 0; i-- {
			if session.StoredFiles[i].Key == key {
				session.StoredFiles[i].Stop = time
				break
			}
		}
	}
	return true
}

// handleSummary handles TraceType_Summary
func (tr *TraceReader) handleSummary(out *TraceView, reader *BinaryReader, time uint64) bool {
	if out.Version < 15 {
		time = reader.Read7BitEncoded()
	} else {
	}
	tr.StopAllActive(out, time)
	return true // Continue processing instead of stopping
}

// handleBeginWork handles TraceType_BeginWork
func (tr *TraceReader) handleBeginWork(out *TraceView, reader *BinaryReader, time uint64) bool {
	var workIndex uint32
	if out.Version < 14 {
		workIndex = reader.ReadU32()
	} else {
		workIndex = uint32(reader.Read7BitEncoded())
	}

	var trackIndex uint32 = 0
	var workTrack *WorkTrack = nil

	// Find available work track
	for i := range out.WorkTracks {
		track := &out.WorkTracks[i]
		if len(track.Records) > 0 && track.Records[len(track.Records)-1].Stop == ^uint64(0) {
			continue
		}
		trackIndex = uint32(i)
		workTrack = track
		break
	}

	if workTrack == nil {
		trackIndex = uint32(len(out.WorkTracks))
		out.WorkTracks = append(out.WorkTracks, WorkTrack{})
		workTrack = &out.WorkTracks[trackIndex]
	}

	workTrack.Records = append(workTrack.Records, WorkRecord{})
	record := &workTrack.Records[len(workTrack.Records)-1]

	tr.activeWorkRecords[workIndex] = WorkRecordLocation{
		Track: trackIndex,
		Index: uint32(len(workTrack.Records) - 1),
	}

	var stringIndex uint64
	if out.Version < 14 {
		stringIndex = uint64(reader.ReadU32())
	} else {
		stringIndex = reader.Read7BitEncoded()
	}

	if int(stringIndex) < len(out.Strings) {
		record.Description = out.Strings[stringIndex]
	}

	if out.Version < 15 {
		record.Start = reader.Read7BitEncoded()
	} else {
		record.Start = time
	}
	record.Stop = ^uint64(0)

	return true
}

// handleEndWork handles TraceType_EndWork
func (tr *TraceReader) handleEndWork(out *TraceView, reader *BinaryReader, time uint64) bool {
	var workIndex uint32
	if out.Version < 14 {
		workIndex = reader.ReadU32()
	} else {
		workIndex = uint32(reader.Read7BitEncoded())
	}

	location, exists := tr.activeWorkRecords[workIndex]
	if !exists {
		blog.Infof("ubatrace: Warning: EndWork for workIndex %d but work record not found, continuing...", workIndex)
		return true // Continue processing instead of stopping
	}
	delete(tr.activeWorkRecords, workIndex)

	record := &out.WorkTracks[location.Track].Records[location.Index]
	if out.Version < 15 {
		record.Stop = reader.Read7BitEncoded()
	} else {
		record.Stop = time
	}

	return true
}

// handleProgressUpdate handles TraceType_ProgressUpdate
func (tr *TraceReader) handleProgressUpdate(out *TraceView, reader *BinaryReader) bool {
	out.ProgressProcessesTotal = uint32(reader.Read7BitEncoded())
	out.ProgressProcessesDone = uint32(reader.Read7BitEncoded())
	out.ProgressErrorCount = uint32(reader.Read7BitEncoded())
	return true
}

// handleStatusUpdate handles TraceType_StatusUpdate
func (tr *TraceReader) handleStatusUpdate(out *TraceView, reader *BinaryReader) bool {
	if out.Version < 32 {
		// Skip old format
		reader.Read7BitEncoded()
		reader.Read7BitEncoded()
		reader.ReadString()
		reader.Read7BitEncoded()
		reader.ReadString()
		reader.ReadByte()
	} else {
		row := reader.Read7BitEncoded()
		column := reader.Read7BitEncoded()
		key := (row << 32) | column

		if out.StatusMap == nil {
			out.StatusMap = make(map[uint64]StatusUpdate)
		}

		text, err := reader.ReadString()
		if err != nil {
			return false
		}

		link, err := reader.ReadString()
		if err != nil {
			return false
		}

		status := StatusUpdate{
			Text: text,
			Type: reader.ReadByte(),
			Link: link,
		}
		out.StatusMap[key] = status
	}
	return true
}

// handleRemoteExecutionDisabled handles TraceType_RemoteExecutionDisabled
func (tr *TraceReader) handleRemoteExecutionDisabled(out *TraceView, reader *BinaryReader) bool {
	out.RemoteExecutionDisabled = true
	return true
}

// handleString handles TraceType_String
func (tr *TraceReader) handleString(out *TraceView, reader *BinaryReader) bool {
	outstr, err := reader.ReadString()
	if err != nil {
		return false
	}
	out.Strings = append(out.Strings, outstr)
	return true
}

// handleCacheBeginFetch handles TraceType_CacheBeginFetch
func (tr *TraceReader) handleCacheBeginFetch(out *TraceView, reader *BinaryReader, time uint64) bool {
	id := uint32(reader.Read7BitEncoded())
	desc, err := reader.ReadString()
	if err != nil {
		return false
	}

	process := tr.ProcessBegin(out, 0, id, time, desc, "")
	if process != nil {
		process.CacheFetch = true
	}
	return true
}

// handleCacheEndFetch handles TraceType_CacheEndFetch
func (tr *TraceReader) handleCacheEndFetch(out *TraceView, reader *BinaryReader, time uint64) bool {
	id := uint32(reader.Read7BitEncoded())
	success := reader.ReadBool()

	process, _ := tr.ProcessEnd(out, id, time)
	if process == nil {
		blog.Infof("ubatrace: Warning: CacheEndFetch for processId %d but process not found, continuing...", id)
		return true
	}

	process.ExitCode = 0
	if !success {
		process.ReturnedReason = "M"
	}

	// Read cache stats according to C++ implementation
	dataStart := reader.GetPosition()

	// Read CacheStats
	cacheStats := reader.ReadCacheStats(out.Version)
	_ = cacheStats // Avoid unused variable warning

	// Read additional stats if success or version >= 29
	if success || out.Version >= 29 {
		storageStats := reader.ReadStorageStats(out.Version)
		kernelStats := reader.ReadKernelStats(out.Version)
		_ = storageStats // Avoid unused variable warning
		_ = kernelStats  // Avoid unused variable warning
	}

	dataEnd := reader.GetPosition()
	statsSize := dataEnd - dataStart
	process.Stats = make([]byte, statsSize)
	// Copy the stats data from the original position
	if statsSize > 0 {
		copy(process.Stats, reader.data[dataStart:dataEnd])
	}

	return true
}

// handleCacheBeginWrite handles TraceType_CacheBeginWrite
func (tr *TraceReader) handleCacheBeginWrite(out *TraceView, reader *BinaryReader, time uint64) bool {
	processID := uint32(reader.Read7BitEncoded())

	if out.CacheWrites == nil {
		out.CacheWrites = make(map[uint32]CacheWrite)
	}

	cacheWrite := out.CacheWrites[processID]
	cacheWrite.Start = time
	out.CacheWrites[processID] = cacheWrite

	return true
}

// handleCacheEndWrite handles TraceType_CacheEndWrite
func (tr *TraceReader) handleCacheEndWrite(out *TraceView, reader *BinaryReader, time uint64) bool {
	processID := uint32(reader.Read7BitEncoded())

	if out.CacheWrites == nil {
		out.CacheWrites = make(map[uint32]CacheWrite)
	}

	cacheWrite := out.CacheWrites[processID]
	cacheWrite.Success = reader.ReadBool()
	cacheWrite.BytesSent = reader.Read7BitEncoded()
	cacheWrite.End = time
	out.CacheWrites[processID] = cacheWrite

	return true
}

func (tr *TraceReader) handleFileFetchSize(out *TraceView, reader *BinaryReader) bool {
	// 读取数据
	clientUid := tr.ReadClientId(out, reader)
	key := reader.ReadCasKey()
	fileSize := reader.Read7BitEncoded()

	// 获取会话
	session := tr.GetSessionByUID(out, clientUid)
	if session == nil {
		return false
	}

	// 查找活跃文件
	fileIndex, exists := session.FetchedFilesActive[key]
	if !exists {
		return false
	}

	// 更新文件大小
	file := session.FetchedFiles[fileIndex]
	if file == (FileTransfer{}) {
		return false
	}

	file.Size = fileSize
	return true
}
type BreadcrumbWriter func(process *Process)

func createBreadcrumbWriter(breadcrumbs string, deleteOld bool) BreadcrumbWriter {
	return func(process *Process) {
		if deleteOld {
			process.Breadcrumbs = ""
		} else if process.Breadcrumbs != "" {
			process.Breadcrumbs += "\n"
		}
		process.Breadcrumbs += string(breadcrumbs)
	}
}

func (tr *TraceReader) handleProcessBreadcrumbs(out *TraceView, reader *BinaryReader) bool {
	// 读取数据
	processId := reader.ReadU32()
	
	var breadcrumbs string
	var err error
	if out.Version < 38 {
		breadcrumbs, err = reader.ReadString()
	} else {
		breadcrumbs, err = reader.ReadLongString()
	}
	if err != nil {
		return false
	}

	deleteOld := reader.ReadBool()

	// 创建面包屑写入函数
	writeBreadcrumb := createBreadcrumbWriter(breadcrumbs, deleteOld)

	// 首先在活动进程中查找
	if activeInfo, exists := tr.activeProcesses[processId]; exists {
		session := tr.GetSession(out, activeInfo.SessionIndex)
		if session == nil {
			return false
		}
		
		if int(activeInfo.ProcessorIndex) >= len(session.Processors) {
			return false
		}
		processor := session.Processors[activeInfo.ProcessorIndex]
		if int(activeInfo.ProcessIndex) >= len(processor.Processes) {
			return false
		}
		
		process := processor.Processes[activeInfo.ProcessIndex]
		writeBreadcrumb(&process)
		return true
	}

	// 如果不在活动进程中，进行全量搜索
	found := false
	for _, session := range out.Sessions {
		for _, processor := range session.Processors {
			for _, process := range processor.Processes {
				if process.ID == processId {
					writeBreadcrumb(&process)
					found = true
					// 注意：这里不立即返回，继续搜索所有匹配的进程
					// 根据业务需求决定是否应该返回
				}
			}
		}
	}

	return found 
}
func (tr *TraceReader) handleWorkHint(out *TraceView, reader *BinaryReader, time uint64) bool {
	// 读取基本数据
	workIndex := uint32(reader.Read7BitEncoded())
	stringIndex := reader.Read7BitEncoded()
	startTimeRaw := reader.Read7BitEncoded()
	startTime := ConvertTime(out, startTimeRaw)

	// 查找工作记录
	active, exists := tr.activeWorkRecords[workIndex]
	if !exists {
		return false
	}

	// 获取文本
	text:= out.Strings[stringIndex]
	if text == "" {
		return false

	}

	// 获取工作记录
	track := out.WorkTracks[active.Track]
	if int(active.Index) >= len(track.Records) {
		return false
	}

	record := track.Records[active.Index]
	handled := false

	// 如果存在开始时间，尝试合并现有条目
	if startTime > 0 {
		// 反向遍历条目（从最新到最旧）
		for i := len(record.Entries) - 1; i >= 0; i-- {
			entry := &record.Entries[i]
			
			// 如果遇到没有开始时间的条目，停止搜索
			if entry.StartTime == 0 {
				break
			}
			
			// 文本不匹配，继续搜索
			if entry.Text != text {
				continue
			}

			// 找到匹配的条目，增加计数并更新时间
			entry.Count++
			entryTime := entry.Time - entry.StartTime
			newTime := time - startTime

			if newTime > entryTime {
				entry.Time = time
				entry.StartTime = startTime
			}

			handled = true
			break
		}
	}

	// 如果没有处理过，添加新条目
	if !handled {
		newEntry := WorkRecordLogEntry{
			Time:      time,
			StartTime: startTime,
			Text:      text,
			Count:     1,
		}
		record.Entries = append(record.Entries, newEntry)
	}

	return true
}

func (tr *TraceReader) handleDriverUpdate(out *TraceView, reader *BinaryReader) bool {
	// 读取驱动数据
	driveLetter := reader.ReadByte()
	busyPercent := reader.ReadByte()
	readCount := reader.Read7BitEncoded()
	readBytes := reader.Read7BitEncoded()
	writeCount := reader.Read7BitEncoded()
	writeBytes := reader.Read7BitEncoded()

	// 检查是否有会话
	if len(out.Sessions) == 0 {
		return true
	}

	session := &out.Sessions[0]

	// 初始化驱动映射
	if session.Drives == nil {
		session.Drives = make(map[uint8]Drive)
	}

	drive, exists := session.Drives[driveLetter]
	if !exists {
		session.Drives[driveLetter] = Drive{}
	}

	// 初始化切片
	if len(drive.BusyPercent) == 0 {
		updatesCount := len(session.Updates)
		drive.BusyPercent = make([]uint8, updatesCount)
		drive.ReadCount = make([]uint32, updatesCount)
		drive.WriteCount = make([]uint32, updatesCount)
		drive.ReadBytes = make([]uint64, updatesCount)
		drive.WriteBytes = make([]uint64, updatesCount)
	}

	// 更新最高繁忙百分比
	if busyPercent > drive.BusyHighest {
		drive.BusyHighest = busyPercent
	}

	// 添加新数据
	drive.BusyPercent = append(drive.BusyPercent, busyPercent)
	drive.TotalReadCount += uint32(readCount)
	drive.TotalWriteCount += uint32(writeCount)
	drive.TotalReadBytes += readBytes
	drive.TotalWriteBytes += writeBytes
	drive.ReadCount = append(drive.ReadCount, uint32(readCount))
	drive.ReadBytes = append(drive.ReadBytes, readBytes)
	drive.WriteCount = append(drive.WriteCount, uint32(writeCount))
	drive.WriteBytes = append(drive.WriteBytes, writeBytes)

	return true
}

func formatTimeUS(timeus int64) string {
	// 将微秒转换为秒和纳秒
	seconds := timeus / 1e6
	nanoseconds := (timeus % 1e6) * 1e3

	// 使用 time.Unix 函数创建 time.Time 对象
	t := time.Unix(seconds, nanoseconds)

	return t.Format("2006-01-02 15:04:05.000000")
}

// ReadUBAFile reads and parses a UBA trace file
func ReadUBAFile(filename string) (*TraceView, error) {
	// Track trace type counts
	traceTypeCounts := make(map[uint8]int)
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Read the entire file into memory
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	blog.Infof("ubatrace: read UBA file: %s (%d bytes)", filename, len(data))

	// Create binary reader
	reader := NewBinaryReader(data)

	// Read the header
	if len(data) < 16 {
		return nil, fmt.Errorf("file too small to contain valid UBA header")
	}

	// Read UBA file header (based on C++ implementation)
	magic := reader.ReadU32()
	blog.Infof("ubatrace: Magic number: 0x%08X", magic)
	if magic != 0x41425555 { // "UBA\0" in little endian
		blog.Infof("ubatrace: Warning: Unexpected magic number: 0x%08X (expected 0x41425555)", magic)
	}

	version := reader.ReadU32()
	blog.Infof("ubatrace: UBA file version: %d", version)

	// Initialize trace view
	traceView := &TraceView{
		Version:     version,
		Sessions:    make([]Session, 0),
		WorkTracks:  make([]WorkTrack, 0),
		Strings:     make([]string, 0),
		StatusMap:   make(map[uint64]StatusUpdate),
		CacheWrites: make(map[uint32]CacheWrite),
		Frequency:   GetFrequency(),
		Finished:    false,
	}

	// Read additional header fields
	processID := reader.ReadU32()
	blog.Infof("ubatrace: Process ID: %d", processID)

	// var traceSystemStartTimeUs uint64 = 0
	if version >= 18 {
		traceView.SystemStartTimeUs = reader.Read7BitEncoded()
		blog.Infof("ubatrace: Trace system start time (us): %d [%s]",
			traceView.SystemStartTimeUs,
			formatTimeUS(int64(traceView.SystemStartTimeUs)))
	}

	if version >= 18 {
		traceView.Frequency = reader.Read7BitEncoded()
	}
	blog.Infof("ubatrace: Frequency: %d", traceView.Frequency)

	traceView.RealStartTime = reader.Read7BitEncoded()
	traceView.StartTime = traceView.RealStartTime
	blog.Infof("ubatrace: Start time: %d", traceView.StartTime)

	// Create trace reader
	traceReader := NewTraceReader()

	// Process all trace entries
	entryCount := 0
	maxTime := ^uint64(0) // Max uint64

	for reader.GetPosition() < uint64(len(data)) {
		entryCount++
		currentPos := reader.GetPosition()

		success := traceReader.ReadTrace(traceView, reader, maxTime, traceTypeCounts)
		if !success {
			blog.Infof("ubatrace: Trace processing failed, stopping")
			break
		}

		// Check if position advanced to prevent infinite loops
		newPos := reader.GetPosition()
		if newPos == currentPos {
			blog.Infof("ubatrace: Position did not advance from %d, stopping to prevent infinite loop", currentPos)
			break
		}
	}

	blog.Infof("ubatrace: finished read UBA file: %s", filename)

	return traceView, nil
}
