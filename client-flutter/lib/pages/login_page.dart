// 登录 / 注册页（班级下拉 30 项：初一1~10班 / 初二1~10班 / 初三1~10班）
import 'package:flutter/material.dart';

import '../api.dart';
import '../main.dart' show kDefaultBaseUrl, prefs;

class LoginPage extends StatefulWidget {
  const LoginPage({super.key, required this.api, required this.onLogin});
  final ApiClient api;
  final VoidCallback onLogin;
  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage>
    with SingleTickerProviderStateMixin {
  late final TabController _tabs = TabController(length: 2, vsync: this);

  final _loginUser = TextEditingController();
  final _loginPass = TextEditingController();
  final _regUser = TextEditingController();
  final _regPass = TextEditingController();
  final _regNick = TextEditingController();
  final _regRealName = TextEditingController();
  final _regStudentNo = TextEditingController();
  String _className = '初一1班';
  String _baseUrl = kDefaultBaseUrl;
  String? _error;
  bool _busy = false;

  static const List<String> _classNames = [
    for (final g in ['初一', '初二', '初三'])
      for (var i = 1; i <= 10; i++) '$g${i}班',
  ];

  @override
  void initState() {
    super.initState();
    _baseUrl = prefs.getString('base_url') ?? kDefaultBaseUrl;
  }

  Future<void> _login() async {
    setState(() { _busy = true; _error = null; });
    try {
      // 允许直接改服务器地址（先改再登录）
      if (_baseUrl != widget.api.baseUrl) {
        widget.api.baseUrl = _baseUrl;
        await prefs.setString('base_url', _baseUrl);
      }
      final user = await widget.api.login(_loginUser.text, _loginPass.text);
      await prefs.setString('token', widget.api.token);
      if (!mounted) return;
      if (user['status'] != 'approved') {
        setState(() {
          _error = '账号状态：${user['status'] == 'pending' ? '等待审核' : user['status']}';
        });
        return;
      }
      widget.onLogin();
    } on ApiException catch (e) {
      setState(() => _error = e.msg);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _register() async {
    setState(() { _busy = true; _error = null; });
    try {
      await widget.api.register(
        username: _regUser.text,
        password: _regPass.text,
        nickname: _regNick.text,
        className: _className,
        realName: _regRealName.text,
        studentNo: _regStudentNo.text,
      );
      if (!mounted) return;
      _tabs.animateTo(0);
      setState(() => _error = '注册成功，等待老师审核');
    } on ApiException catch (e) {
      setState(() => _error = e.msg);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 400),
            child: Card(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text('揭东一初联机大厅',
                        textAlign: TextAlign.center,
                        style: Theme.of(context).textTheme.headlineSmall),
                    const SizedBox(height: 4),
                    Text('实名注册，人工审核后开放',
                        textAlign: TextAlign.center,
                        style: Theme.of(context).textTheme.bodySmall),
                    const SizedBox(height: 16),
                    TabBar(controller: _tabs, tabs: const [
                      Tab(text: '登录'), Tab(text: '注册'),
                    ]),
                    const SizedBox(height: 16),
                    SizedBox(
                      height: _tabs.index == 0 ? 230 : 360,
                      child: TabBarView(controller: _tabs, children: [
                        _buildLogin(), _buildRegister(),
                      ]),
                    ),
                    if (_error != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: Text(_error!,
                            style: TextStyle(
                                color: Theme.of(context).colorScheme.error)),
                      ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _field(TextEditingController c, String hint, {bool obscure = false}) =>
      Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: TextField(
          controller: c,
          obscureText: obscure,
          decoration: InputDecoration(hintText: hint, border: const OutlineInputBorder()),
        ),
      );

  Widget _buildLogin() => Column(children: [
        _field(_loginUser, '用户名'),
        _field(_loginPass, '密码', obscure: true),
        _field(TextEditingController(text: _baseUrl), '服务器地址'),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: _busy ? null : _login,
          child: const Text('登录'),
        ),
      ]);

  Widget _buildRegister() => ListView(shrinkWrap: true, children: [
        _field(_regUser, '用户名（4-20 位字母数字下划线）'),
        _field(_regPass, '密码（8-64 位，含字母和数字）', obscure: true),
        _field(_regNick, '昵称'),
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: DropdownButtonFormField<String>(
            value: _className,
            decoration: const InputDecoration(
                labelText: '班级', border: OutlineInputBorder()),
            items: [
              for (final c in _classNames)
                DropdownMenuItem(value: c, child: Text(c)),
            ],
            onChanged: (v) => setState(() => _className = v ?? _className),
          ),
        ),
        _field(_regRealName, '真实姓名（2-4 个汉字）'),
        _field(_regStudentNo, '学号（选填）'),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: _busy ? null : _register,
          child: const Text('提交注册'),
        ),
      ]);

  @override
  void dispose() {
    _tabs.dispose();
    for (final c in [_loginUser, _loginPass, _regUser, _regPass, _regNick, _regRealName, _regStudentNo]) {
      c.dispose();
    }
    super.dispose();
  }
}
