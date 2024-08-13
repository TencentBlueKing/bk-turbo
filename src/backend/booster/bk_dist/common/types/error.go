/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package types

import "fmt"

type BKDistCommonError struct {
	Code  int
	Error error
}

var (
	DescPreForceLocal       = fmt.Errorf("cmd in force local when pre execute")
	DescPreNotSupportRemote = fmt.Errorf("cmd not support remote when pre execute")

	DescRemoteSendFile      = fmt.Errorf("send file failed when remote execute")
	DescRemoteSendToolchain = fmt.Errorf("send toolchain failed when remote execute")

	DescPostMissPumpDependFile   = fmt.Errorf("miss pump depend file when post execute")
	DescPostMissNormalDependFile = fmt.Errorf("miss normal depend file when post execute")
	DescPostSaveFileFailed       = fmt.Errorf("save file failed when post execute")
	DescPostOutOfMemoy           = fmt.Errorf("out of memory when post execute")
	DescPostToolchainNotFound    = fmt.Errorf("toolchain not found when post execute")

	DescUnknown = fmt.Errorf("unknown")
)

// define errors
var (
	// no error        0
	ErrorNone = BKDistCommonError{Code: 0, Error: nil}

	// pre-execute     1~999
	ErrorPreForceLocal       = BKDistCommonError{Code: 1, Error: DescPreForceLocal}
	ErrorPreNotSupportRemote = BKDistCommonError{Code: 2, Error: DescPreNotSupportRemote}

	// remote-execute  1000~1999
	ErrorRemoteSendFile      = BKDistCommonError{Code: 1000, Error: DescRemoteSendFile}
	ErrorRemoteSendToolchain = BKDistCommonError{Code: 1001, Error: DescRemoteSendToolchain}

	// post-execute    2000~2999
	ErrorPostMissPumpDependFile   = BKDistCommonError{Code: 2000, Error: DescPostMissPumpDependFile}
	ErrorPostMissNormalDependFile = BKDistCommonError{Code: 2001, Error: DescPostMissNormalDependFile}
	ErrorPostSaveFileFailed       = BKDistCommonError{Code: 2002, Error: DescPostSaveFileFailed}
	ErrorPostOutOfMemoy           = BKDistCommonError{Code: 2100, Error: DescPostOutOfMemoy}
	ErrorPostToolchainNotFound    = BKDistCommonError{Code: 2200, Error: DescPostToolchainNotFound}

	// local-execute   3000~3999

	// other error     999999
	ErrorUnknown = BKDistCommonError{Code: 999999, Error: DescUnknown}
)
