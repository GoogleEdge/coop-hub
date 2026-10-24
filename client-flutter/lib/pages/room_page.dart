// 房间内页：连接信息、frpc/edge 启动、日志、离开
// UI 文案与联机参数全部来自服务端下发的 room_joined 载荷。
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api.dart';
import '../ffi_core.dart';

class RoomPage extends StatefulWidget {
  const RoomPage({super.key, required this.api, required this.joined});
  final ApiClient api;
  final Map<String, dynamic> joined; // WS room_joined 载荷
  @override
  State<RoomPage> createState() => _RoomPageState();
}

class _RoomPageState extends State<RoomPage> {
  List<CoreLogLine> _logs = [];
  String? _error;
  bool _started = false;

  Map<String, dynamic>? get _frp =>
      (widget.joined['frp'] as Map?)?.cast<String, dynamic>();
  Map<String, dynamic>? get _vlan =>
      (widget.joined['vlan'] as Map?)?.cast<String, dynamic>();
  bool get _isVlan => _vlan != null;

  String get _appDir => File(Platform.resolvedExecutable).parent.path;

  @override
  void initState() {
    super.initState();
    JdycCore.onLog((line) {
      if (mounted) setState(() => _logs = JdycCore.logs);
    });
    WidgetsBinding.instance.addPostFrameCallback((_) => _autoStart());
  }

  Future<void> _autoStart() async {
    if (_isVlan) {
      final core = JdycCore.instance();
      if (core.tapInstalled() == -10) {
        final go = await showDialog<bool>(
          context: context,
          builder: (_) => AlertDialog(
            title: const Text('需要安装虚拟网卡驱动'),
            content: const Text('首次使用虚拟局域网需要安装 TAP 网卡驱动。\n'
                '点击「安装」后请在弹出的 UAC 窗口选择「是」。'),
            actions: [
              TextButton(
                  onPressed: () => Navigator.pop(context, false),
                  child: const Text('取消')),
              FilledButton(
                  onPressed: () => Navigator.pop(context, true),
                  child: const Text('安装')),
            ],
          ),
        );
        if (go != true) {
          _leave();
          return;
        }
        final rc = core.tapInstall(_appDir);
        if (rc != 0) {
          setState(() => _error = 'TAP 驱动安装失败（错误码 $rc）');
          return;
        }
      }
      _startVlan();
    } else {
      _startFrp();
    }
  }

  // 生成 frpc 最小 TOML（frps 参数全部来自服务端下发，不硬编码）
  Future<File> _writeFrpToml(Map<String, dynamic> f) async {
    String proxy(String name, String type) =>
        '[[proxies]]\nname = "$name"\ntype = "$type"\n'
        'localPort = ${f['local_port']}\nremotePort = ${f['remote_port']}\n\n';
    var toml = 'serverAddr = "${f['frpc_server']}"\n'
        'serverPort = ${f['frpc_port']}\n'
        'auth.token = "${f['frpc_token']}"\n\n';
    final base = f['proxy_name']?.toString() ?? 'jdyc';
    switch (f['port_type']) {
      case 'tcp':
        toml += proxy('${base}-tcp', 'tcp');
        break;
      case 'udp':
        toml += proxy('${base}-udp', 'udp');
        break;
      default: // both
        toml += proxy('${base}-tcp', 'tcp') + proxy('${base}-udp', 'udp');
    }
    final path =
        '${Directory.systemTemp.path}\\jdyc-frp-${f['remote_port']}.toml';
    return File(path).writeAsString(toml);
  }

  Future<void> _startFrp() async {
    final f = _frp!;
    final toml = await _writeFrpToml(f);
    final rc = JdycCore.instance().frpStart('$_appDir\\frpc.exe', toml.path);
    setState(() {
      _started = rc == 0;
      if (rc != 0) _error = 'frpc 启动失败（错误码 $rc）';
    });
  }

  Future<void> _startVlan() async {
    final v = _vlan!;
    final rc = JdycCore.instance().vlanStart('$_appDir\\edge.exe',
        v['network_community'], v['vlan_key'], v['supernode'], v['my_virt_ip']);
    setState(() {
      _started = rc == 0;
      if (rc != 0) _error = 'edge 启动失败（错误码 $rc）';
    });
  }

  Future<void> _leave() async {
    // 停子进程 + WS 退房（RoomPage 返回即断开，服务端按非房主即时退房）
    final core = JdycCore.instanceSafe();
    if (core != null) {
      if (_isVlan) {
        core.vlanStop();
      } else {
        core.frpStop();
      }
    }
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final j = widget.joined;
    return Scaffold(
      appBar: AppBar(
        title: Text('房间：${j['room_name'] ?? ''}'),
        actions: [
          IconButton(icon: const Icon(Icons.exit_to_app), onPressed: _leave),
        ],
      ),
      body: ListView(padding: const EdgeInsets.all(16), children: [
        if (_isVlan) ..._buildVlanCards() else ..._buildFrpCards(),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(top: 12),
            child: Text(_error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
          ),
        const SizedBox(height: 12),
        Text('子进程日志', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        Container(
          height: 220,
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: const Color(0xFF17191F),
            borderRadius: BorderRadius.circular(8),
          ),
          child: ListView.builder(
            itemCount: _logs.length,
            itemBuilder: (_, i) {
              final l = _logs[i];
              return Text(
                l.line,
                style: TextStyle(
                  fontFamily: 'Consolas',
                  fontSize: 12,
                  color: l.level == 2
                      ? const Color(0xFFE06C60)
                      : l.level == 1
                          ? const Color(0xFFE5C07B)
                          : const Color(0xFF9AA0AA),
                ),
              );
            },
          ),
        ),
      ]),
    );
  }

  List<Widget> _buildFrpCards() {
    final f = _frp!;
    final addr = '${f['frpc_server']}:${f['remote_port']}';
    return [
      Card(
        child: ListTile(
          title: const Text('在游戏中直接连接此地址'),
          subtitle: Text(addr),
          trailing: IconButton(
            icon: const Icon(Icons.copy),
            onPressed: () async {
              await Clipboard.setData(ClipboardData(text: addr));
              if (mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('已复制到剪贴板')));
              }
            },
          ),
        ),
      ),
    ];
  }

  List<Widget> _buildVlanCards() {
    final v = _vlan!;
    return [
      Card(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('虚拟局域网已就绪',
                  style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 8),
              Text('本机虚拟 IP：${v['my_virt_ip']}'),
              Text('房主虚拟 IP：${v['host_virt_ip']}（在游戏里连这个地址）'),
              Text('社区名：${v['network_community']}'),
            ],
          ),
        ),
      ),
    ];
  }
}
