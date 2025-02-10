package sdk

import (
	"fmt"
	"net"
	"os"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// RemoteWorker describe the remote worker SDK
type RemoteWorker interface {
	Handler(
		ioTimeout int,
		stats *ControllerJobStats,
		updateJobStatsFunc func(),
		sandbox *syscall.Sandbox) RemoteWorkerHandler
}

// LockMgr to support lock
type LockMgr interface {
	// lock local slot
	LockSlots(usage JobUsage, weight int32) bool

	// unlock local slot
	UnlockSlots(usage JobUsage, weight int32)
}

// RemoteWorkerHandler describe the remote worker handler SDK
type RemoteWorkerHandler interface {
	ExecuteSyncTime(server string) (int64, error)
	ExecuteTask(server *dcProtocol.Host, req *BKDistCommand) (*BKDistResult, error)
	ExecuteTaskWithoutSaveFile(server *dcProtocol.Host, req *BKDistCommand) (*BKDistResult, error)
	ExecuteSendFile(
		server *dcProtocol.Host,
		req *BKDistFileSender,
		sandbox *syscall.Sandbox,
		mgr LockMgr) (*BKSendFileResult, error)
	ExecuteCheckCache(server *dcProtocol.Host, req *BKDistFileSender, sandbox *syscall.Sandbox) ([]bool, error)

	// with long tcp connection
	ExecuteTaskWithoutSaveFileLongTCP(server *dcProtocol.Host, req *BKDistCommand) (*BKDistResult, error)
	ExecuteTaskLongTCP(server *dcProtocol.Host, req *BKDistCommand) (*BKDistResult, error)
	ExecuteSendFileLongTCP(
		server *dcProtocol.Host,
		req *BKDistFileSender,
		sandbox *syscall.Sandbox,
		mgr LockMgr) (*BKSendFileResult, error)
	ExecuteCheckCacheLongTCP(server *dcProtocol.Host, req *BKDistFileSender, sandbox *syscall.Sandbox) ([]bool, error)
	ExecuteSyncTimeLongTCP(server string) (int64, error)

	ExecuteQuerySlot(
		server *dcProtocol.Host,
		req *BKQuerySlot,
		c chan *BKQuerySlotResult,
		timeout int) (*net.TCPConn, error)

	ExecuteReportResultCache(
		server *dcProtocol.Host,
		attributes map[string]string,
		results []*FileDesc) (*BKReportResultCacheResult, error)

	ExecuteQueryResultCacheIndex(
		server *dcProtocol.Host,
		attributes map[string]string) (*BKQueryResultCacheIndexResult, error)

	ExecuteQueryResultCacheFile(
		server *dcProtocol.Host,
		attributes map[string]string) (*BKQueryResultCacheFileResult, error)
}

// FileDescPriority from 0 ~ 100, from high to low
type FileDescPriority int

const (
	MinFileDescPriority FileDescPriority = 100
	MaxFileDescPriority FileDescPriority = 0

	// pump cache模式，文件发送需要保证顺序，定义下面几个优先级(由高到底)
	RealDirPriority  FileDescPriority = 0
	LinkDirPriority  FileDescPriority = 1
	RealFilePriority FileDescPriority = 2
	LinkFilePriority FileDescPriority = 3

	// 优先级先只定3个
	PriorityLow     = 0
	PriorityMiddle  = 1
	PriorityHight   = 2
	PriorityUnKnown = 99
)

// FileDesc desc file base info
type FileDesc struct {
	FilePath           string                `json:"file_path"`
	FileSize           int64                 `json:"file_size"`
	InitFileSize       int64                 `json:"init_file_size"`
	Lastmodifytime     int64                 `json:"last_modify_time"`
	Md5                string                `json:"md5"`
	Compresstype       protocol.CompressType `json:"compress_type"`
	Buffer             []byte                `json:"buffer"`
	CompressedSize     int64                 `json:"compressed_size"`
	InitCompressedSize int64                 `json:"init_compressed_size"`
	Targetrelativepath string                `json:"target_relative_path"`
	Filemode           uint32                `json:"file_mode"`
	LinkTarget         string                `json:"link_target"`
	NoDuplicated       bool                  `json:"no_duplicated"`
	AllDistributed     bool                  `json:"all_distributed"`
	Priority           FileDescPriority      `json:"priority"`
	Retry              bool                  `json:"retry"`
}

// UniqueKey define the file unique key
func (f *FileDesc) UniqueKey() string {
	return fmt.Sprintf("%s_%d_%d", f.FilePath, f.FileSize, f.Lastmodifytime)
}

// FileResult desc file base info
type FileResult struct {
	FilePath           string `json:"file_path"`
	RetCode            int32  `json:"ret_code"`
	Targetrelativepath string `json:"target_relative_path"`
}

// BKCommand command to execute
type BKCommand struct {
	WorkDir         string     `json:"work_dir"`
	ExePath         string     `json:"exe_path"`
	ExeName         string     `json:"exe_name"`
	ExeToolChainKey string     `json:"exe_toolchain_key"`
	Params          []string   `json:"params"`
	Inputfiles      []FileDesc `json:"input_files"`
	ResultFiles     []string   `json:"result_files"`
	Env             []string   `json:"env"`
}

// Result result after execute command
type Result struct {
	RetCode       int32      `json:"ret_code"`
	OutputMessage []byte     `json:"output_message"`
	ErrorMessage  []byte     `json:"error_message"`
	ResultFiles   []FileDesc `json:"result_files"`
}

// BKDistCommand set by handler
type BKDistCommand struct {
	Commands []BKCommand `json:"commands"`

	// messages are the raw command ready-to-send data
	Messages []protocol.Message `json:"messages"`

	CustomSave bool `json:"custom_save"` // whether save result file custom
}

// BKDistFileSender describe the files sending to worker
type BKDistFileSender struct {
	Files []FileDesc `'json:"file"`

	Messages []protocol.Message `json:"messages"`
}

// BKDistResult return to handler
type BKDistResult struct {
	Results []Result `json:"results"`
}

// BKSendFileResult return to handler
type BKSendFileResult struct {
	Results []FileResult `json:"file_results"`
}

// BKReportResultCacheResult result for report result cache
type BKReportResultCacheResult struct {
	RetCode       int32  `json:"retcode"`
	OutputMessage string `json:"output_message"`
	ErrorMessage  string `json:"error_message"`
}

// BKQueryResultCacheIndexResult result for query result cache index
type BKQueryResultCacheIndexResult struct {
	ResultIndex []byte `'json:"result_index"`
}

// BKQueryResultCacheFileResult result for query result cache file
type BKQueryResultCacheFileResult struct {
	Resultfiles []FileDesc `'json:"result_files"`
}

// LocalTaskResult
type LocalTaskResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   []byte `json:"stdout"`
	Stderr   []byte `json:"stderr"`
	Message  string `json:"message"`
}

// BKQuerySlot
type BKQuerySlot struct {
	Priority         int32  `json:"priority"`
	WaitTotalTaskNum int32  `json:"wait_total_task_num"`
	TaskType         string `json:"task_type"`
}

// BKQuerySlotResult
type BKQuerySlotResult struct {
	Host             *dcProtocol.Host `json:"host"`
	Priority         int32            `json:"priority"`
	AvailableSlotNum int32            `json:"available_slot_num"`
	Refused          int32            `json:"refused"`
	Message          string           `json:"message"`
}

// BKSlotRspAck
type BKSlotRspAck struct {
	Consumeslotnum int32 `json:"consume_slot_num"`
}

func GetPriority(i *dcFile.Info) FileDescPriority {
	switch i.FileType {
	case file.RealDir:
		return RealDirPriority
	case file.LinkDir:
		return LinkDirPriority
	case file.RealFile:
		return RealFilePriority
	case file.LinkFile:
		return LinkFilePriority
	}

	isLink := i.Basic().Mode()&os.ModeSymlink != 0
	if !isLink {
		if i.Basic().IsDir() {
			return RealDirPriority
		} else {
			return RealFilePriority
		}
	}

	// symlink 需要判断是指向文件还是目录
	if i.LinkTarget != "" {
		targetfs, err := dcFile.GetFileInfo([]string{i.LinkTarget}, true, false, false)
		if err == nil && len(targetfs) > 0 {
			if targetfs[0].Basic().IsDir() {
				return LinkDirPriority
			} else {
				return LinkFilePriority
			}
		}
	}

	// 尝试读文件
	linkpath := i.Path()
	targetPath, err := os.Readlink(linkpath)
	if err != nil {
		blog.Infof("common util: Error reading symbolic link: %v", err)
		return LinkFilePriority
	}

	// 获取符号链接指向路径的文件信息
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		blog.Infof("common util: Error getting target file info: %v", err)
		return LinkFilePriority
	}

	// 判断符号链接指向的路径是否是目录
	if targetInfo.IsDir() {
		blog.Infof("common util: %s is a symbolic link to a directory", linkpath)
		return LinkDirPriority
	} else {
		blog.Infof("common util: %s is a symbolic link, but not to a directory", linkpath)
		return LinkFilePriority
	}
}
