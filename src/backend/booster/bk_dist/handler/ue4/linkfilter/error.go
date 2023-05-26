/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package linkfilter

import "fmt"

// errors for link-filter.exe
var (
	ErrorMissingOption  = fmt.Errorf("missing option/operand")
	ErrorInvalidParam   = fmt.Errorf("param is invalid")
	ErrorNilInnerHandle = fmt.Errorf("inner handle is nil")
)
