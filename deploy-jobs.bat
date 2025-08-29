@echo off
for %%x in (%*) do (
    echo deploying job %%~x
    rem nomad job plan %%~x
    rem if ERRORLEVEL 1 nomad job run %%~x
)
exit /b 0