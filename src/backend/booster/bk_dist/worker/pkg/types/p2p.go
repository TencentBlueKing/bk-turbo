/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package types

// Resource : 资源信息
type Resource struct {
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
}

// AgentBase : agent info
type AgentBase struct {
	IP      string            `json:"ip"`
	Port    int               `json:"port"`
	Message string            `json:"message"`
	Cluster string            `json:"cluster"`
	Labels  map[string]string `json:"labels"`
}

// AgentInfo : agent info
type AgentInfo struct {
	Base  AgentBase `json:"base"`
	Total Resource  `json:"total"`
	Free  Resource  `json:"free"`
}

// ReportAgentResource : struct of report resource
type ReportAgentResource struct {
	AgentInfo
}
