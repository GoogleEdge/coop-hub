// api.dart：REST 客户端（register/login/me/rooms）
import 'dart:convert';
import 'package:http/http.dart' as http;

class ApiException implements Exception {
  final int statusCode;
  final String code;
  final String msg;
  ApiException(this.statusCode, this.code, this.msg);
  @override
  String toString() => msg.isEmpty ? '请求失败（$statusCode）' : msg;
}

class ApiClient {
  ApiClient({required this.baseUrl});
  String baseUrl; // 允许登录页修改
  String token = '';

  Map<String, String> get _headers => {
        'Content-Type': 'application/json; charset=utf-8',
        if (token.isNotEmpty) 'Authorization': 'Bearer $token',
      };

  Future<dynamic> _send(String method, String path, Object? body) async {
    final uri = Uri.parse('$baseUrl$path');
    late http.Response resp;
    try {
      switch (method) {
        case 'POST':
          resp = await http.post(uri,
              headers: _headers,
              body: body == null ? '' : jsonEncode(body));
          break;
        case 'GET':
          resp = await http.get(uri, headers: _headers);
          break;
        case 'DELETE':
          resp = await http.delete(uri, headers: _headers);
          break;
        default:
          throw ApiException(0, 'bad_method', '不支持的请求方法');
      }
    } catch (e) {
      if (e is ApiException) rethrow;
      throw ApiException(0, 'network', '无法连接服务器，请检查网络');
    }
    dynamic data;
    try {
      data = resp.body.isEmpty ? {} : jsonDecode(utf8.decode(resp.bodyBytes));
    } catch (_) {
      data = {};
    }
    if (resp.statusCode >= 400) {
      final map = data is Map ? data : {};
      throw ApiException(resp.statusCode, map['error']?.toString() ?? '',
          map['msg']?.toString() ?? '请求失败');
    }
    return data;
  }

  // 注册：班级下拉 30 项在 UI 层生成
  Future<void> register({
    required String username,
    required String password,
    required String nickname,
    required String className,
    required String realName,
    required String studentNo,
  }) =>
      _send('POST', '/api/register', {
        'username': username,
        'password': password,
        'nickname': nickname,
        'class_name': className,
        'real_name': realName,
        'student_no': studentNo,
      }).then((_) {});

  // 登录：返回用户信息（含 status，用于提示审核中）
  Future<Map<String, dynamic>> login(String username, String password) async {
    final data = await _send(
        'POST', '/api/login', {'username': username, 'password': password});
    token = data['token']?.toString() ?? '';
    final user = (data['user'] as Map?)?.cast<String, dynamic>() ?? {};
    return user;
  }

  Future<Map<String, dynamic>> me() async =>
      (await _send('GET', '/api/me', null) as Map).cast<String, dynamic>();

  Future<List<dynamic>> listRooms() async {
    final data = await _send('GET', '/api/rooms', null);
    return (data['rooms'] as List? ?? []);
  }

  Future<Map<String, dynamic>> createRoom({
    required String name,
    required String game,
    required String mode,
    int maxMembers = 20,
    int? localPort,
    String? portType,
  }) async {
    final data = await _send('POST', '/api/rooms', {
      'name': name,
      'game': game,
      'mode': mode,
      'max_members': maxMembers,
      if (localPort != null) 'local_port': localPort,
      if (portType != null) 'port_type': portType,
    });
    return (data['room'] as Map).cast<String, dynamic>();
  }

  Future<void> deleteRoom(int roomId) =>
      _send('DELETE', '/api/rooms/$roomId', null).then((_) {});
}
