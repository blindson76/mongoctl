@echo off
taskkill -f -im mongod.exe
taskkill /f /im nomad.exe
taskkill /f /im consul.exe
taskkill /f /fi "WINDOWTITLE eq Node-*"
for %%l in (1 3 6) do (
    start /min cmd /c start-agent.bat %%l %1
)