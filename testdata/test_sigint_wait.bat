@echo off

set wait_time=%1

rem Run infinite loop in another cmd shell, which should run until someone hits CTRL+C
rem For more info, see: https://stackoverflow.com/a/28890881/483528
cmd /d /c %~dp0infinite_loop.bat

sleep %wait_time%
exit %wait_time%