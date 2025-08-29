@echo off
set CONSOLE_ID=%1
set NODE_ID=node-%CONSOLE_ID%
set CSB_IP=10.10.5%CONSOLE_ID%.1
title Node-%CONSOLE_ID%
echo this is %CONSOLE_ID%
set PATH=%~dp0;%PATH%
set PATH=%~dp0cots\mongo;%PATH%
setlocal EnableDelayedExpansion
set NOMAD_DATA_DIR=%~dp0data\nomad\%CONSOLE_ID%
set MONGO_DB_PATH=%~dp0data\mongo\%CONSOLE_ID%
set CONSUL_DATA_DIR=%~dp0data\consul\%CONSOLE_ID%
set NOMAD_ADDR=http://%CSB_IP%:4646
set CONSUL_HTTP_ADDR=http://%CSB_IP%:8500
set MONGO_PORT=27017
set MONGO_ADDR=%CSB_IP%
set MONGO_LOCAL_ADDR=%CSB_IP%

set RS_NAME=rs0
set CLASSPATH=%~dp0\target\*;%~dp0\target\lib\*;
rmdir /S /Q %NOMAD_DATA_DIR%
rmdir /S /Q %CONSUL_DATA_DIR%
rem rmdir /S /Q %MONGO_DB_PATH%
mkdir %NOMAD_DATA_DIR%
mkdir data\mongo\%CONSOLE_ID%
rem node wait.js 1
echo starting consul
start "consul-%CONSOLE_ID%" /min cmd /C consul agent -server -ui -config-file consul.hcl -data-dir %CONSUL_DATA_DIR% -node %NODE_ID% -client %CSB_IP% -bind %CSB_IP% -bootstrap-expect 3 -retry-join 10.10.51.1  -retry-join 10.10.52.1  -retry-join 10.10.53.1  -retry-join 10.10.54.1  -retry-join 10.10.55.1  -retry-join 10.10.56.1
start "nomad-%CONSOLE_ID%" /min cmd /c nomad agent -server -config client.hcl -config client.hcl -bind %CSB_IP% -consul-address %CSB_IP%:8500 -data-dir %NOMAD_DATA_DIR% -network-interface loop -node %NODE_ID% -bootstrap-expect 3 -retry-join 10.10.51.1:4648 -retry-join 10.10.52.1:4648 -retry-join 10.10.53.1:4648 -retry-join 10.10.54.1:4648 -retry-join 10.10.55.1:4648 -retry-join 10.10.56.1:4648
node wait_nomad.js %NOMAD_ADDR%
go run -C goctl . -prestart
pause
echo nomad ready
goto exit
nomad node meta apply -unset role.mongo
nomad var purge status/mongo/%NODE_ID%
java com.example.MongoPrestart
:exit
echo done
rem pause
set SHELL=cmd.exe
@REM nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy /c nomad job run jobs\mongo\mongo.hcl
@REM nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy2 /c nomad job run jobs\mongo\mongo-member.hcl
