@echo off
set "PATH=C:\Users\ubozkurt\Downloads\jdk-24_windows-x64_bin\jdk-24.0.1\bin;%PATH%"
set "JAVA_HOME=C:\Users\ubozkurt\Downloads\jdk-24_windows-x64_bin\jdk-24.0.1"
taskkill -f -im mongod.exe
taskkill /f /im nomad.exe
taskkill /f /im consul.exe
taskkill /f /fi "WINDOWTITLE eq Node-*"
for %%l in (2 4 5) do (
    start /min cmd /c start-agent.bat %%l
)