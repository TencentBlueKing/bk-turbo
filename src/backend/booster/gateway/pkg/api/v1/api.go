/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package v1

import (
	"sync"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg/api"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg/api/v1/apisjob"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg/api/v1/distcc"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg/api/v1/disttask"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg/api/v1/fastbuild"

	"github.com/emicklei/go-restful"
)

var one sync.Once

// After server init, the instances of manager, store ... etc. should be given into api handler.
func InitStorage() (err error) {
	one.Do(func() {
		api.RegisterV1Action(api.Action{
			Verb:    "GET",
			Path:    "/health",
			Params:  nil,
			Handler: api.NoLimit(health),
		})

		initDCCActions()
		initFBActions()
		initAPISActions()
		initDistTaskActions()
		initAutoDistTaskActions()
	})
	return nil
}

// health return ok to caller
func health(_ *restful.Request, resp *restful.Response) {
	api.ReturnRest(&api.RestResponse{Resp: resp})
}

func initDCCActions() {
	if api.GetDistCCServerAPIResource().MySQL == nil {
		return
	}

	// distcc task
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/task",
		Params:  nil,
		Handler: api.AuthRequired(distcc.ListTask),
	})
	// distcc worker images
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/images",
		Params:  nil,
		Handler: api.AuthRequired(distcc.ListWorkerImages),
	})
	// distcc project
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/project",
		Params:  nil,
		Handler: api.AuthRequired(distcc.ListProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/distcc/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(distcc.UpdateProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/distcc/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(distcc.DeleteProject),
	})

	// distcc whitelist
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(distcc.ListWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/distcc/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(distcc.UpdateWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/distcc/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(distcc.DeleteWhitelist),
	})

	// distcc gcc
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/gcc",
		Params:  nil,
		Handler: api.AuthRequired(distcc.ListGcc),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/distcc/resource/gcc/{gcc_version}",
		Params:  nil,
		Handler: api.AuthRequired(distcc.UpdateGcc),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/distcc/resource/gcc/{gcc_version}",
		Params:  nil,
		Handler: api.AuthRequired(distcc.DeleteGcc),
	})

	// distcc summary statistics
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/summary",
		Params:  nil,
		Handler: api.AuthRequired(distcc.Summary),
	})

	// distcc private cluster summary statistics
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/distcc/resource/summary/private",
		Params:  nil,
		Handler: api.AuthRequired(distcc.SummaryPrivate),
	})
}

func initFBActions() {
	if api.GetFBServerAPIResource().MySQL == nil {
		return
	}

	// fb task
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/fb/resource/task",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.ListTask),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/fb/resource/subtask",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.ListSubTask),
	})

	// fb project
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/fb/resource/project",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.ListProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/fb/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.UpdateProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/fb/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.DeleteProject),
	})

	// fb whitelist
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/fb/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.ListWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/fb/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.UpdateWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/fb/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(fastbuild.DeleteWhitelist),
	})
}

func initAPISActions() {
	if api.GetXNAPISServerAPIResource().MySQL == nil {
		return
	}

	// apis task
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/apisjob/resource/task",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.ListTask),
	})

	// apis project
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/apisjob/resource/project",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.ListProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/apisjob/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.UpdateProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/apisjob/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.DeleteProject),
	})

	// apis whitelist
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/apisjob/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.ListWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/apisjob/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.UpdateWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/apisjob/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(apisjob.DeleteWhitelist),
	})
}

func initDistTaskActions() {
	if api.GetDistTaskServerAPIResource().MySQL == nil {
		return
	}
	// disttask client version
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/version",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListClientVersion),
	})

	// disttask worker images
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/images",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListWorkerImages),
	})

	// disttask task
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/task",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListTask),
	})

	// disttask work stats
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/stats",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListWorkStats),
	})

	// disttask project
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/project",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/disttask/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.UpdateProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/disttask/resource/project/{project_id}/scene/{scene}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.UpdateProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/disttask/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.DeleteProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/disttask/resource/project/{project_id}/scene/{scene}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.DeleteProject),
	})

	// disttask whitelist
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/disttask/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(disttask.UpdateWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/disttask/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(disttask.DeleteWhitelist),
	})

	// disttask worker
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/worker",
		Params:  nil,
		Handler: api.AuthRequired(disttask.ListWorker),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    "/disttask/resource/worker/{worker_version}/scene/{scene}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.UpdateWorker),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    "/disttask/resource/worker/{worker_version}/scene/{scene}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.DeleteWorker),
	})

	// disttask summary statistics
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/summary",
		Params:  nil,
		Handler: api.AuthRequired(disttask.Summary),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/summary/groupbyuser/scene/{scene}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.SummaryByUser),
	})

	// disttask private cluster summary statistics
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    "/disttask/resource/summary/private",
		Params:  nil,
		Handler: api.AuthRequired(disttask.SummaryPrivate),
	})
}
func initAutoDistTaskActions() {
	if api.GetDistTaskServerAPIResource().MySQL == nil {
		return
	}

	autoKey := "/{auto_scene:disttask-[0-9A-Za-z_]+}"

	// auto disttask task
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    autoKey + "/resource/task",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoListTask),
	})

	// auto disttask work stats
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    autoKey + "/resource/stats",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoListWorkStats),
	})

	// auto disttask task
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    autoKey + "/resource/project",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoListProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    autoKey + "/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoUpdateProject),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    autoKey + "/resource/project/{project_id}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoDeleteProject),
	})

	// auto disttask whitelist
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    autoKey + "/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoListWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    autoKey + "/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoUpdateWhitelist),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    autoKey + "/resource/whitelist",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoDeleteWhitelist),
	})

	// auto disttask worker
	api.RegisterV1Action(api.Action{
		Verb:    "GET",
		Path:    autoKey + "/resource/worker",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoListWorker),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "PUT",
		Path:    autoKey + "/resource/worker/{worker_version}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoUpdateWorker),
	})
	api.RegisterV1Action(api.Action{
		Verb:    "DELETE",
		Path:    autoKey + "/resource/worker/{worker_version}",
		Params:  nil,
		Handler: api.AuthRequired(disttask.AutoDeleteWorker),
	})
}

func init() {
	api.RegisterInitFunc(InitStorage)
}
