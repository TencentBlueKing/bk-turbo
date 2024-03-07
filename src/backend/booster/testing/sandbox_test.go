package testing

import (
	"testing"

	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
)

func TestSandBox(t *testing.T) {
	sandbox := dcSyscall.Sandbox{}
	d, err := sandbox.ExecScripts("ls -lha")
	if err != nil {
		t.Error(err)
	}
	if d != 0 {
		t.Error("expect return code == 0")
	}
}
