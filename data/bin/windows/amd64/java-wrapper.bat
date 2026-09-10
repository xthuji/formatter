@echo off
setlocal enabledelayedexpansion

REM java-wrapper.bat - Windows 等价于 java-wrapper (bash)
REM 包装 google-java-format.jar 以支持 stdin/stdout Java 格式化

set "VERSION=1.1.0"

if "%~1"=="--version" (
    echo java-wrapper %VERSION%
    exit /b 0
)
if "%~1"=="-v" (
    echo java-wrapper %VERSION%
    exit /b 0
)

REM JAR 查找: 同目录 (打包后) → 上级 ..\..\ (源码树 data/bin/windows/amd64 → data/bin/)
set "script_dir=%~dp0"
set "JAR=!script_dir!google-java-format.jar"
if not exist "!JAR!" (
    set "JAR=!script_dir!..\..\google-java-format.jar"
)
if not exist "!JAR!" (
    echo java-wrapper: google-java-format.jar not found >&2
    exit /b 1
)

REM 从 stdin 读取到临时文件
set "tmpfile=%TEMP%\jfw_%RANDOM%.java"
more > "%tmpfile%"

REM 格式化
java -jar "!JAR!" --aosp "%tmpfile%" > "%tmpfile%.out" 2>nul
if !errorlevel! neq 0 (
    echo java-wrapper: format failed >&2
    del "%tmpfile%" 2>nul
    del "%tmpfile%.out" 2>nul
    exit /b 1
)

type "%tmpfile%.out"

REM 清理
del "%tmpfile%" 2>nul
del "%tmpfile%.out" 2>nul
