@echo off
taskkill /F /IM qo_server.exe /T 2>nul
start /B "" qo_server.exe
timeout /t 2 /nobreak > nul
start /B /max http://localhost:8080