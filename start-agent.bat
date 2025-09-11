@echo off
set CONSOLE_ID=%1
set NODE_ID=%CONSOLE_ID%
set NODE_NAME=node-%CONSOLE_ID%
set CSB_IP=10.10.5%CONSOLE_ID%.1
title Node-%CONSOLE_ID%
echo this is %CONSOLE_ID%
set PATH=%~dp0;%PATH%
set PATH=%~dp0\cots;%PATH%
set PATH=%~dp0cots\mongo;%PATH%
set CMS_ROOT=%~dp0
setlocal EnableDelayedExpansion
set NOMAD_DATA_DIR=%~dp0data\nomad\%CONSOLE_ID%
set MONGO_DB_PATH=%~dp0data\mongo\%CONSOLE_ID%
set CONSUL_DATA_DIR=%~dp0data\consul\%CONSOLE_ID%
set NOMAD_ADDR=http://%CSB_IP%:14646
set CONSUL_HTTP_ADDR=http://%CSB_IP%:8500
set MONGO_PORT=27017
set MONGO_ADDR=%CSB_IP%
set MONGO_LOCAL_ADDR=%CSB_IP%


set KAFKA_DATA_DIR=%~dp0data\kafka\%CONSOLE_ID%\data
set KAFKA_META_DIR=%~dp0data\kafka\%CONSOLE_ID%\meta
set KAFKA_LOG_DIR=%~dp0data\kafka\%CONSOLE_ID%\log

set KAFKA_CLUSTER_ID=b_ue-dU-TrybRneDxGS_Ow
if "%CONSOLE_ID%"=="1" (
    set KAFKA_STORAGE_ID=279THXvBR4WGfBj_y1nstQ
)
if "%CONSOLE_ID%"=="2" (
    set KAFKA_STORAGE_ID=TMoKnP9HRz-NVmhcEVuSGQ
)
if "%CONSOLE_ID%"=="3" (
    set KAFKA_STORAGE_ID=x1AWfhMTQ7u_isq4a04w6Q
)
if "%CONSOLE_ID%"=="4" (
    set KAFKA_STORAGE_ID=mP3P8tD3RqSCn5ytqs6Sqw
)
if "%CONSOLE_ID%"=="5" (
    set KAFKA_STORAGE_ID=dJrECDp5Rue3vxpq51EaLQ
)
if "%CONSOLE_ID%"=="6" (
    set KAFKA_STORAGE_ID=Bhml8362S5eGsFfP7o7noQ
)

set RS_NAME=rs0
set CLASSPATH=%~dp0\target\*;%~dp0\target\lib\*;
rmdir /S /Q %NOMAD_DATA_DIR%
rmdir /S /Q %CONSUL_DATA_DIR%
rem rmdir /S /Q %MONGO_DB_PATH%
mkdir %NOMAD_DATA_DIR%
mkdir data\mongo\%CONSOLE_ID%
rem node wait.js 1
echo starting consul
start "consul-%CONSOLE_ID%" /min cmd /C consul agent -server -ui -config-file consul.hcl -data-dir %CONSUL_DATA_DIR% -node %NODE_NAME% -client %CSB_IP% -bind %CSB_IP% -bootstrap-expect 3 -retry-join 10.10.51.1  -retry-join 10.10.52.1  -retry-join 10.10.53.1  -retry-join 10.10.54.1  -retry-join 10.10.55.1  -retry-join 10.10.56.1
start "nomad-%CONSOLE_ID%" /min cmd /C nomad agent -server -config client.hcl -config server.hcl -bind %CSB_IP% -consul-address %CSB_IP%:8500 -data-dir %NOMAD_DATA_DIR% -network-interface loop -node %NODE_NAME% -bootstrap-expect 3 -retry-join 10.10.51.1:14648 -retry-join 10.10.52.1:14648 -retry-join 10.10.53.1:14648 -retry-join 10.10.54.1:14648 -retry-join 10.10.55.1:14648 -retry-join 10.10.56.1:14648
node wait_nomad.js %NOMAD_ADDR%
echo nomad ready!
echo starting mongo-control job
nomad run -detach %CMS_ROOT%\jobs\mongo\mongo-control.hcl && echo ok.
go run -C goctl . -prestart > prestart-%NODE_ID%.log 2>&1
echo nomad ready
rem start cmd /k title %NODE_NAME%

goto exit
nomad node meta apply -unset role.mongo
nomad var purge status/mongo/%NODE_NAME%
java com.example.MongoPrestart
:exit
echo done
rem pause
set SHELL=cmd.exe
@REM nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy /c nomad job run jobs\mongo\mongo.hcl
@REM nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy2 /c nomad job run jobs\mongo\mongo-member.hcl
