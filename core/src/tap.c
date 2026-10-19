/* tap.c：TAP-Windows6 (tap0901) 检测与静默安装。 */

#include <windows.h>
#include <stdio.h>
#include <string.h>
#include "jdyc_core.h"
#include "core_internal.h"

/* TAP 设备类 GUID（网卡类） */
static const WCHAR *NET_CLASS = L"SYSTEM\\CurrentControlSet\\Control\\Class\\{4D36E972-E325-11CE-BFC1-08002BE10318}";

/* 枚举网卡类下所有子键，找 ComponentId 以 "tap0901" 开头的设备 */
int jdyc_tap_check_registry(void)
{
    HKEY hKey;
    DWORD idx = 0;
    WCHAR name[256];
    DWORD nameLen = 256;
    LONG rc;

    if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, NET_CLASS, 0, KEY_READ, &hKey) != ERROR_SUCCESS)
        return -10;
    for (;;) {
        nameLen = 256;
        rc = RegEnumKeyExW(hKey, idx++, name, &nameLen, NULL, NULL, NULL, NULL);
        if (rc != ERROR_SUCCESS) break;
        {
            HKEY sub;
            WCHAR compId[256];
            DWORD sz = sizeof(compId);
            WCHAR path[512];
            lstrcpyW(path, NET_CLASS);
            lstrcatW(path, L"\\");
            lstrcatW(path, name);
            if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, path, 0, KEY_READ, &sub) == ERROR_SUCCESS) {
                DWORD type;
                if (RegQueryValueExW(sub, L"ComponentId", NULL, &type,
                                     (LPBYTE)compId, &sz) == ERROR_SUCCESS) {
                    compId[255] = 0;
                    /* tap0901 大小写不敏感前缀匹配 */
                    WCHAR lower[256];
                    DWORD i;
                    for (i = 0; i < 256 && compId[i]; i++) {
                        WCHAR c = compId[i];
                        lower[i] = (c >= L'A' && c <= L'Z') ? (c + 32) : c;
                    }
                    lower[i] = 0;
                    if (wcsncmp(lower, L"tap0901", 7) == 0) {
                        RegCloseKey(sub);
                        RegCloseKey(hKey);
                        return 0;
                    }
                }
                RegCloseKey(sub);
            }
        }
    }
    RegCloseKey(hKey);
    return -10;
}

int jdyc_tap_installed(void)
{
    return jdyc_tap_check_registry();
}

/* 静默安装：运行安装器等待结束。NSIS 包用 /S；NSIS 需管理员权限（客户端已以管理员运行）。 */
int jdyc_tap_install(const char* installer_path)
{
    STARTUPINFOW si;
    PROCESS_INFORMATION pi;
    wchar_t* exe;
    wchar_t* cmd;
    DWORD exitCode = 1;
    size_t len;

    if (!installer_path) return -1;
    if (jdyc_tap_check_registry() == 0) return 0; /* 已装无需再装 */

    exe = jdyc_utf8_to_wide(installer_path);
    if (!exe) return -1;
    len = lstrlenW(exe) + 8;
    cmd = (wchar_t*)HeapAlloc(GetProcessHeap(), 0, len * sizeof(WCHAR));
    if (!cmd) { HeapFree(GetProcessHeap(), 0, exe); return -1; }
    _snwprintf(cmd, len, L"%s /S", exe);

    ZeroMemory(&si, sizeof(si));
    si.cb = sizeof(si);
    ZeroMemory(&pi, sizeof(pi));
    if (!CreateProcessW(NULL, cmd, NULL, NULL, FALSE,
                        CREATE_NO_WINDOW, NULL, NULL, &si, &pi)) {
        HeapFree(GetProcessHeap(), 0, exe);
        HeapFree(GetProcessHeap(), 0, cmd);
        return -11;
    }
    WaitForSingleObject(pi.hProcess, 120000); /* 最多等 2 分钟 */
    if (GetExitCodeProcess(pi.hProcess, &exitCode) && exitCode != 0) {
        CloseHandle(pi.hProcess);
        CloseHandle(pi.hThread);
        HeapFree(GetProcessHeap(), 0, exe);
        HeapFree(GetProcessHeap(), 0, cmd);
        if (jdyc_tap_check_registry() == 0) return 0;
        return -11;
    }
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);
    HeapFree(GetProcessHeap(), 0, exe);
    HeapFree(GetProcessHeap(), 0, cmd);
    return jdyc_tap_check_registry();
}
