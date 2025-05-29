@echo off
set CONSOLE_ID=%1
set NODE_ID=node-%CONSOLE_ID%
set CSB_IP=10.10.11.1
title Node-%CONSOLE_ID%
echo this is %CONSOLE_ID%
set PATH=%~dp0;%PATH%
set PATH=%~dp0cots\mongo;%PATH%
setlocal EnableDelayedExpansion
set DATA_DIR=%~dp0data\nomad\%CONSOLE_ID%
set DB_PATH=%~dp0data\mongo\%CONSOLE_ID%
set NOMAD_ADDR=http://%CSB_IP%:4%CONSOLE_ID%46
set MONGO_PORT=2701%CONSOLE_ID%
set MONGO_ADDR=%CSB_IP%
set RS_NAME=rs0

rmdir /S /Q %DATA_DIR%
rmdir /S /Q %DB_PATH%
mkdir %DATA_DIR%%
mkdir data\mongo\%CONSOLE_ID%
node wait.js 5
start "nomad-%CONSOLE_ID%" /min cmd /C nomad agent -config nomad-%CONSOLE_ID%.hcl -data-dir %DATA_DIR% -node %NODE_ID% -bootstrap-expect 3 -retry-join 10.10.11.1:4148 -retry-join 10.10.11.1:4248 -retry-join 10.10.11.1:4348 -retry-join 10.10.11.1:4448 -retry-join 10.10.11.1:4548 -retry-join 10.10.11.1:4648
node wait_nomad.js %NOMAD_ADDR%
set SHELL=cmd.exe
nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy /c nomad job run jobs\mongo\mongo.hcl 
@REM nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy /c nomad job run jobs\mongo\mongo-member.hcl
