echo "windows compile"
@echo off
SET curpath=%cd%

set "encryption_key=%1"
set server_cert_pwd=
set client_cert_pwd=

for /F %%i in ('git describe --tags') do ( set gittag=%%i)
echo GITTAG=%gittag%

if [%BUILDTIME%] == [] (
    rem set BUILDTIME=%date:~0,4%-%date:~5,2%-%date:~8,2%
    set BUILDTIME=%date:~3,4%.%date:~8,2%.%date:~11,2%
)

for /F %%i in ('git rev-parse HEAD') do ( set githash=%%i)
echo GITHASH=%githash%

if [%CURRENTDATE%] == [] (
    rem set CURRENTDATE=%date:~0,4%.%date:~5,2%.%date:~8,2%
    set CURRENTDATE=%date:~3,4%.%date:~8,2%.%date:~11,2%
)

set VERSION=%GITTAG%-%CURRENTDATE%
echo %VERSION%

set "LDFLAG1=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.EncryptionKey=%encryption_key%"
set "LDFLAG2=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.ServerCertPwd=%server_cert_pwd%"
set "LDFLAG3=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.ClientCertPwd=%client_cert_pwd%"
set "LDFLAG4=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.InnerIPClassA=%inner_ip_classa%"
set "LDFLAG5=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.InnerIPClassA1=%inner_ip_classa1%"
set "LDFLAG6=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.InnerIPClassAa=%inner_ip_classaa%"
set "LDFLAG7=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.InnerIPClassB=%inner_ip_classb%"
set "LDFLAG8=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static.InnerIPClassC=%inner_ip_classc%"
set "LDFLAG9=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/version.Version=%VERSION%"
set "LDFLAG10=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/version.BuildTime=%BUILDTIME%"
set "LDFLAG11=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/version.GitHash=%GITHASH%"
set "LDFLAG12=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/version.Tag=%GITTAG%"
set "LDFLAG=%LDFLAG1% %LDFLAG2% %LDFLAG3% %LDFLAG4% %LDFLAG5% %LDFLAG6% %LDFLAG7% %LDFLAG8% %LDFLAG9% %LDFLAG10% %LDFLAG11% %LDFLAG12%"

set "BuildBooster_LDFLAG1=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/booster/command.ProdBuildBoosterServerDomain=%distcc_server_prod_domain%"
set "BuildBooster_LDFLAG2=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/booster/command.ProdBuildBoosterServerPort=%distcc_server_prod_port%"
set "BuildBooster_LDFLAG3=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/booster/command.TestBuildBoosterServerDomain=%distcc_server_test_domain%"
set "BuildBooster_LDFLAG4=-X github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/booster/command.TestBuildBoosterServerPort=%distcc_server_test_port%"
set "BuildBooster_LDFLAG=%BuildBooster_LDFLAG1% %BuildBooster_LDFLAG2% %BuildBooster_LDFLAG3% %BuildBooster_LDFLAG4%"

cd %curpath%
set bindir=%curpath%\bin
if not exist %bindir% (
	mkdir %bindir%
)

go build -ldflags "%LDFLAG%" -o %bindir%\bk-bb-agent.exe %curpath%\server\pkg\resource\direct\agent\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-booster.exe %curpath%\bk_dist\booster\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-idle-loop.exe %curpath%\bk_dist\idleloop\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-dist-monitor.exe %curpath%\bk_dist\monitor\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-help-tool.exe %curpath%\bk_dist\helptool\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-ubt-tool.exe %curpath%\bk_dist\ubttool\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-shader-tool.exe %curpath%\bk_dist\shadertool\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-dist-controller.exe %curpath%\bk_dist\controller\main.go

go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG%" -o %bindir%\bk-dist-executor.exe %curpath%\bk_dist\executor\main.go

set "Hide_LDFLAG=-H=windowsgui"
go build -ldflags "%LDFLAG% %BuildBooster_LDFLAG% %Hide_LDFLAG%" -o %bindir%/bk-dist-worker.exe %curpath%\bk_dist\worker\main.go