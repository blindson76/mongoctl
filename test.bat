@echo off

:run
setlocal enabledelayedexpansion
set "vals=1 2 3 4 5 6"
set sets=""


for %%s in (%VALS%) do (
    for /f %%r in ('cmd /c echo !RANDOM!') do (
        set val_[%%r]=%%s
    )
)
set count=0
set "selected="
for /f %%s in ('set val_') do (
    set /a count=!count!+1
    if !count! gtr 3 goto :break
    for /f "delims==, tokens=2" %%c in ('echo %%s') do set "selected=!selected! %%c"
)
:break
for /f "tokens=* delims= " %%A in ("!selected!") do set final=%%A
taskkill -f -im node.exe
taskkill -f -im mongod.exe
echo !final!
start cmd /k
start "node-ctl" /wait cmd /c node mongo_ctrl.js !final!
rem for /l %%e in ('set val_') do echo %%e
rem goto run
rem for %%K in (!final!) do start_db.bat %%K > nul 2>&1
echo starting node ctrl
echo done

