// 大厅页：房间列表（轮询刷新）+ 筛选 + 建房 + WebSocket 房间事件
import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../api.dart';
import 'room_page.dart';

class LobbyPage extends StatefulWidget {
  const LobbyPage({super.key, required this.api, required this.onLogout});
  final ApiClient api;
  final VoidCallback onLogout;
  @override
  State<LobbyPage> createState() => _LobbyPageState();
}

class _LobbyPageState extends State<LobbyPage> {
  List<dynamic> _rooms = [];
  String _filter = '';
  WebSocketChannel? _ws;
  Timer? _refreshTimer;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    _refresh();
    _connectWs();
    _refreshTimer = Timer.periodic(const Duration(seconds: 10), (_) => _refresh());
  }

  Uri get _wsUri {
    final base = widget.api.baseUrl
        .replaceFirst(RegExp(r'^http'), 'ws');
    return Uri.parse('$base/lobby?token=${Uri.encodeComponent(widget.api.token)}');
  }

  void _connectWs() {
    _ws?.sink.close();
    final channel = WebSocketChannel.connect(_wsUri);
    _ws = channel;
    channel.stream.listen((msg) {
      Map<String, dynamic> m;
      try {
        m = (jsonDecode(msg as String) as Map).cast<String, dynamic>();
      } catch (_) {
        return;
      }
      if (!mounted) return;
      switch (m['type']) {
        case 'room_joined':
          Navigator.of(context).push(MaterialPageRoute(
              builder: (_) => RoomPage(api: widget.api, joined: m)));
          break;
        case 'room_full':
        case 'error':
          ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text(m['msg']?.toString() ?? '操作失败')));
          break;
        default:
          break; // room_list / room_created / room_removed 等走轮询刷新
      }
    }, onError: (_) {}, onDone: () {
      // 断线 5 秒后重连
      Future.delayed(const Duration(seconds: 5), () {
        if (mounted) _connectWs();
      });
    });
  }

  Future<void> _refresh() async {
    try {
      final rooms = await widget.api.listRooms();
      if (mounted) setState(() => _rooms = rooms);
    } on ApiException {
      // 网络错误不打断界面
    }
  }

  String _gameName(String g) => const {
        'minecraft': '我的世界 Java 版',
        'terraria': '泰拉瑞亚',
        'cs16': '反恐精英 1.6',
        'yuzu': 'Switch 模拟器',
        'custom': '其他',
      }[g] ?? g;

  Future<void> _createRoom() async {
    final result = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (_) => _CreateRoomDialog(),
    );
    if (result == null) return;
    setState(() => _busy = true);
    try {
      final room = await widget.api.createRoom(
        name: result['name'] as String,
        game: result['game'] as String,
        mode: result['mode'] as String,
        maxMembers: result['max_members'] as int,
        localPort: result['local_port'] as int?,
        portType: result['port_type'] as String?,
      );
      // 建房成功 → 加入自己的房间，服务端回 room_joined 后进房间页
      _ws?.sink.add(jsonEncode({'type': 'join_room', 'room_id': room['id']}));
    } on ApiException catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(e.msg)));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _join(Map<String, dynamic> room) async {
    _ws?.sink.add(jsonEncode({'type': 'join_room', 'room_id': room['id']}));
  }

  @override
  Widget build(BuildContext context) {
    final shown = _rooms.where((r) {
      if (_filter.isEmpty) return true;
      final s = _filter.toLowerCase();
      return r['name'].toString().toLowerCase().contains(s) ||
          r['game'].toString().toLowerCase().contains(s);
    }).toList();
    return Scaffold(
      appBar: AppBar(
        title: const Text('大厅'),
        actions: [
          IconButton(icon: const Icon(Icons.logout), onPressed: widget.onLogout),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _busy ? null : _createRoom,
        icon: const Icon(Icons.add),
        label: const Text('建房'),
      ),
      body: Column(children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: TextField(
            decoration: const InputDecoration(
                prefixIcon: Icon(Icons.search),
                hintText: '按房间名或游戏筛选',
                border: OutlineInputBorder()),
            onChanged: (v) => setState(() => _filter = v),
          ),
        ),
        Expanded(
          child: shown.isEmpty
              ? const Center(child: Text('暂无房间，点右下角「建房」开一局'))
              : ListView.builder(
                  padding: const EdgeInsets.only(bottom: 88),
                  itemCount: shown.length,
                  itemBuilder: (_, i) {
                    final r = shown[i];
                    final mode = r['mode']?.toString() ?? '';
                    return ListTile(
                      leading: Icon(mode == 'vlan' ? Icons.lan : Icons.router,
                          color: Theme.of(context).colorScheme.primary),
                      title: Text(r['name']?.toString() ?? ''),
                      subtitle: Text(
                          '${_gameName(r['game']?.toString() ?? '')} · ${mode == 'vlan' ? '虚拟局域网' : '端口转发'} · '
                          '${r['members']}/${r['max_members']} 人 · 房主 ${r['host_nickname']}'),
                      trailing: mode == 'frp'
                          ? Text(r['frp_addr']?.toString() ?? '')
                          : null,
                      onTap: () => _join(r),
                    );
                  },
                ),
        ),
      ]),
    );
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    _ws?.sink.close();
    super.dispose();
  }
}

// 建房对话框
class _CreateRoomDialog extends StatefulWidget {
  @override
  State<_CreateRoomDialog> createState() => _CreateRoomDialogState();
}

class _CreateRoomDialogState extends State<_CreateRoomDialog> {
  final _name = TextEditingController(text: '开黑');
  String _game = 'minecraft';
  String _mode = 'frp';
  int _maxMembers = 20;
  final _localPort = TextEditingController(text: '25565');
  String _portType = 'tcp';

  static const _games = {
    'minecraft': '我的世界 Java 版',
    'terraria': '泰拉瑞亚',
    'cs16': '反恐精英 1.6',
    'yuzu': 'Switch 模拟器',
    'custom': '其他',
  };
  // 各游戏默认端口（frp 本机端口）
  static const _defaultPorts = {
    'minecraft': '25565', 'terraria': '7777', 'cs16': '27015',
    'yuzu': '24472', 'custom': '25565',
  };

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('新建房间'),
      content: SizedBox(
        width: 360,
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          TextField(controller: _name,
              decoration: const InputDecoration(labelText: '房间名')),
          DropdownButtonFormField<String>(
            value: _game,
            decoration: const InputDecoration(labelText: '游戏'),
            items: [
              for (final e in _games.entries)
                DropdownMenuItem(value: e.key, child: Text(e.value)),
            ],
            onChanged: (v) => setState(() {
              _game = v!;
              _localPort.text = _defaultPorts[_game]!;
            }),
          ),
          DropdownButtonFormField<String>(
            value: _mode,
            decoration: const InputDecoration(
                labelText: '联机模式',
                helperText: '端口转发=有公网服务器中转；虚拟局域网=TAP 虚拟网卡'),
            items: const [
              DropdownMenuItem(value: 'frp', child: Text('端口转发（FRP）')),
              DropdownMenuItem(value: 'vlan', child: Text('虚拟局域网（n2n）')),
            ],
            onChanged: (v) => setState(() => _mode = v!),
          ),
          Row(children: [
            const Text('人数上限 '),
            Expanded(
              child: Slider(
                value: _maxMembers.toDouble(),
                min: 2, max: 100, divisions: 98,
                label: '$_maxMembers',
                onChanged: (v) => setState(() => _maxMembers = v.round()),
              ),
            ),
          ]),
          if (_mode == 'frp') ...[
            TextField(controller: _localPort,
                decoration: const InputDecoration(labelText: '本机游戏端口')),
            DropdownButtonFormField<String>(
              value: _portType,
              decoration: const InputDecoration(labelText: '转发协议'),
              items: const [
                DropdownMenuItem(value: 'tcp', child: Text('TCP')),
                DropdownMenuItem(value: 'udp', child: Text('UDP')),
                DropdownMenuItem(value: 'both', child: Text('TCP+UDP')),
              ],
              onChanged: (v) => setState(() => _portType = v!),
            ),
          ],
        ]),
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('取消')),
        FilledButton(
          onPressed: () => Navigator.pop(context, {
            'name': _name.text,
            'game': _game,
            'mode': _mode,
            'max_members': _maxMembers,
            if (_mode == 'frp') 'local_port': int.tryParse(_localPort.text) ?? 0,
            if (_mode == 'frp') 'port_type': _portType,
          }),
          child: const Text('创建'),
        ),
      ],
    );
  }
}
