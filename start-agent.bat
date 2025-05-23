@echo off
title Node-%1
echo this is %1
setlocal EnableDelayedExpansion
set DATA_DIR=%~dp0data\nomad\%1
set DB_PATH=%~dp0data\mongo\%1
set NOMAD_ADDR=http://10.10.11.1:4%146
set PATH=%~dp0cots\mongo;%PATH%
::rmdir /S /Q %DATA_DIR%
mkdir %DATA_DIR%%
mkdir data\mongo\%1
node wait.js 15
start "nomad-%1" /min cmd /C nomad agent -config nomad-%1.hcl -data-dir %DATA_DIR% -node node-%1 -bootstrap-expect 3 -retry-join 10.10.11.1:4148 -retry-join 10.10.11.1:4248 -retry-join 10.10.11.1:4348 -retry-join 10.10.11.1:4448 -retry-join 10.10.11.1:4548 -retry-join 10.10.11.1:4648
node wait_nomad.js %NOMAD_ADDR%
set SHELL=cmd.exe
nomad var lock -verbose -ttl=10s -max-retry=1 job/deploy /c nomad job run java-job.hcl 
