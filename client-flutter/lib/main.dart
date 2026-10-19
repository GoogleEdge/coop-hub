// 揭东一初联机大厅（主流版）入口
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'api.dart';
import 'pages/login_page.dart';
import 'pages/lobby_page.dart';

// 服务端地址：默认值需与部署一致，可在登录页修改（本地保存）
const String kDefaultBaseUrl = 'https://jdyc.example.com';

late SharedPreferences prefs;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  prefs = await SharedPreferences.getInstance();
  runApp(const JdycHubApp());
}

class JdycHubApp extends StatelessWidget {
  const JdycHubApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: '揭东一初联机大厅',
      theme: ThemeData(
        useMaterial3: true,
        colorSchemeSeed: const Color(0xFF2E5AAC),
        visualDensity: VisualDensity.standard,
      ),
      home: const RootGate(),
    );
  }
}

class RootGate extends StatefulWidget {
  const RootGate({super.key});
  @override
  State<RootGate> createState() => _RootGateState();
}

class _RootGateState extends State<RootGate> {
  late final ApiClient api;

  @override
  void initState() {
    super.initState();
    api = ApiClient(
        baseUrl: prefs.getString('base_url') ?? kDefaultBaseUrl);
    api.token = prefs.getString('token') ?? '';
  }

  @override
  Widget build(BuildContext context) {
    final hasToken = api.token.isNotEmpty;
    return hasToken
        ? LobbyPage(api: api, onLogout: _logout)
        : LoginPage(api: api, onLogin: _onLogin);
  }

  void _onLogin() {
    setState(() {}); // token 已由 LoginPage 写入 prefs，切到大厅
  }

  void _logout() async {
    await prefs.remove('token');
    api.token = '';
    if (mounted) setState(() {});
  }
}
