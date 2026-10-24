// ffi_core.dart：jdyc-core.dll 的 Dart FFI 绑定
// 与 core/include/jdyc_core.h 的 C ABI 一一对应。
import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';

// typedef void (*jdyc_log_cb)(int level, const char* line)
typedef LogCbNative = Void Function(Int32 level, Pointer<Utf8> line);
typedef LogCbDart = void Function(int level, Pointer<Utf8> line);

typedef StartNative = Int32 Function(Pointer<Utf8>, Pointer<Utf8>, Pointer<NativeFunction<LogCbNative>>);
typedef StartDart = int Function(Pointer<Utf8>, Pointer<Utf8>, Pointer<NativeFunction<LogCbNative>>);

typedef VlanStartNative = Int32 Function(
    Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>,
    Pointer<NativeFunction<LogCbNative>>);
typedef VlanStartDart = int Function(
    Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>, Pointer<Utf8>,
    Pointer<NativeFunction<LogCbNative>>);

typedef SimpleNative = Int32 Function();
typedef SimpleDart = int Function();

typedef TapInstallNative = Int32 Function(Pointer<Utf8>);
typedef TapInstallDart = int Function(Pointer<Utf8>);

class CoreLogLine {
  final int level; // 0=INFO 1=WARN 2=ERROR
  final String line;
  CoreLogLine(this.level, this.line);
}

class JdycCore {
  JdycCore._();
  static JdycCore? _instance;
  late final DynamicLibrary _lib;
  late final StartDart _frpStart;
  late final SimpleDart _frpStop;
  late final SimpleDart _frpRunning;
  late final VlanStartDart _vlanStart;
  late final SimpleDart _vlanStop;
  late final SimpleDart _vlanRunning;
  late final SimpleDart _tapInstalled;
  late final TapInstallDart _tapInstall;

  static bool _cbAlive = false;
  static NativeCallable<LogCbNative>? _cb;

  static JdycCore instance() {
    if (_instance != null) return _instance!;
    final core = JdycCore._();
    core._lib = DynamicLibrary.open('jdyc-core.dll');
    core._frpStart = core._lib
        .lookupFunction<StartNative, StartDart>('jdyc_frp_start');
    core._frpStop =
        core._lib.lookupFunction<SimpleNative, SimpleDart>('jdyc_frp_stop');
    core._frpRunning = core._lib
        .lookupFunction<SimpleNative, SimpleDart>('jdyc_frp_running');
    core._vlanStart = core._lib
        .lookupFunction<VlanStartNative, VlanStartDart>('jdyc_vlan_start');
    core._vlanStop =
        core._lib.lookupFunction<SimpleNative, SimpleDart>('jdyc_vlan_stop');
    core._vlanRunning = core._lib
        .lookupFunction<SimpleNative, SimpleDart>('jdyc_vlan_running');
    core._tapInstalled = core._lib
        .lookupFunction<SimpleNative, SimpleDart>('jdyc_tap_installed');
    core._tapInstall = core._lib
        .lookupFunction<TapInstallNative, TapInstallDart>('jdyc_tap_install');
    _instance = core;
    return core;
  }

  static Pointer<NativeFunction<LogCbNative>> _logCbPointer() {
    _cb?.close();
    _cb = NativeCallable<LogCbNative>.listener((int level, Pointer<Utf8> line) {
      final text = line.toDartString();
      _lastLogs.add(CoreLogLine(level, text));
      if (_lastLogs.length > 500) _lastLogs.removeAt(0);
      _onLog?.call(_lastLogs.last);
      _cbAlive = true;
    });
    return _cb!.nativeFunction;
  }

  static final List<CoreLogLine> _lastLogs = [];
  static void Function(CoreLogLine)? _onLog;
  static void onLog(void Function(CoreLogLine) fn) => _onLog = fn;
  static List<CoreLogLine> get logs => List.unmodifiable(_lastLogs);

  static String _toC(String s) => s.toNativeUtf8();

  // 返回码：0成功 -1参数空 -2已在运行 -3启动失败（另见 jdyc_core.h）
  int frpStart(String frpcPath, String tomlPath) {
    final a = _toC(frpcPath), b = _toC(tomlPath);
    final rc = _frpStart(a, b, _logCbPointer());
    calloc.free(a);
    calloc.free(b);
    return rc;
  }

  int vlanStart(String edgePath, String community, String key,
      String supernode, String myIp) {
    final a = _toC(edgePath),
        b = _toC(community),
        c = _toC(key),
        d = _toC(supernode),
        e = _toC(myIp);
    final rc = _vlanStart(a, b, c, d, e, _logCbPointer());
    calloc
      ..free(a)
      ..free(b)
      ..free(c)
      ..free(d)
      ..free(e);
    return rc;
  }

  // TAP：0已装 -10未装；安装返回 -11 失败 / 0 成功
  // tapDir 为驱动文件目录（tapinstall.exe / OemVista.inf 等所在目录）
  int tapInstalled() => _tapInstalled();
  int tapInstall(String tapDir) {
    final p = _toC(tapDir);
    final rc = _tapInstall(p);
    calloc.free(p);
    return rc;
  }
  int frpStop() => _frpStop();
  int vlanStop() => _vlanStop();
  bool frpRunning() => _frpRunning() != 0;
  bool vlanRunning() => _vlanRunning() != 0;
}
