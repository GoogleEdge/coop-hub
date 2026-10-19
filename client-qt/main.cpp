// 揭东一初联机大厅 Win7 专用版（Qt 5.15 QML）
// jdyc-core.dll 通过 QLibrary 动态加载；QML 页面经 qrc 内嵌，引导页用内存字符串加载。

#include <QApplication>
#include <QClipboard>
#include <QDebug>
#include <QDir>
#include <QFile>
#include <QGuiApplication>
#include <QLibrary>
#include <QMetaObject>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QSettings>

#include "jdyc_core.h"

class AppController;

static AppController* g_controller = nullptr;

// 日志回调（来自子进程读线程）→ 转发到 Qt 主线程
static void core_log_cb(int level, const char* line)
{
    if (!g_controller || !line) return;
    QString msg = QString::fromUtf8(line);
    QMetaObject::invokeMethod(g_controller, "onCoreLog",
                              Qt::QueuedConnection,
                              Q_ARG(int, level), Q_ARG(QString, msg));
}

class AppController : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QString token READ token WRITE setToken NOTIFY tokenChanged)
    Q_PROPERTY(QString baseUrl READ baseUrl CONSTANT)
    Q_PROPERTY(bool frpRunning READ frpRunning NOTIFY runningChanged)
    Q_PROPERTY(bool vlanRunning READ vlanRunning NOTIFY runningChanged)

public:
    explicit AppController(QObject* parent = nullptr) : QObject(parent)
    {
        m_settings = new QSettings("jdyc", "JDYCHub", this);
        m_token = m_settings->value("account/token").toString();
        // 加载与本程序同目录的 jdyc-core.dll
        m_core.setFileName(appDir() + "/jdyc-core.dll");
        if (!m_core.load()) {
            emit appError("jdyc-core.dll 加载失败：" + m_core.errorString());
        }
        resolveFuncs();
    }

    QString token() const { return m_token; }
    void setToken(const QString& t) {
        if (m_token == t) return;
        m_token = t;
        m_settings->setValue("account/token", t);
        emit tokenChanged();
    }
    QString baseUrl() const {
        // 服务端地址集中配置（QSettings 可覆盖），默认占位符
        return m_settings->value("server/baseUrl", "https://jdyc.example.com").toString();
    }
    bool frpRunning() const { return m_frpRunning; }
    bool vlanRunning() const { return m_vlanRunning; }

    Q_INVOKABLE int tapInstalled() {
        if (!m_tapInstalled) return -10;
        return m_tapInstalled();
    }
    Q_INVOKABLE int tapInstall() {
        if (!m_tapInstall) return -11;
        QString exe = appDir() + "/tap-windows-installer.exe";
        QByteArray p = exe.toUtf8();
        return m_tapInstall(p.constData());
    }
    Q_INVOKABLE int frpStart(const QString& toml) {
        if (!m_frpStart || !m_frpStop) return -1;
        if (m_frpRunning) { m_frpStop(); m_frpRunning = false; }
        writeTempFile("jdyc-frp.toml", toml);
        QString frpc = appDir() + "/frpc.exe";
        QByteArray p = frpc.toUtf8();
        QByteArray t = (QDir::tempPath() + "/jdyc-frp.toml").toUtf8();
        int rc = m_frpStart(p.constData(), t.constData(), &core_log_cb);
        m_frpRunning = (rc == 0);
        emit runningChanged();
        return rc;
    }
    Q_INVOKABLE int frpStop() {
        if (!m_frpStop) return -1;
        int rc = m_frpStop();
        m_frpRunning = false;
        emit runningChanged();
        return rc;
    }
    Q_INVOKABLE int vlanStart(const QString& community, const QString& key,
                              const QString& supernode, const QString& myIp) {
        if (!m_vlanStart || !m_vlanStop) return -1;
        if (m_vlanRunning) { m_vlanStop(); m_vlanRunning = false; }
        QString edge = appDir() + "/edge.exe";
        QByteArray a1 = edge.toUtf8(), a2 = community.toUtf8(),
                   a3 = key.toUtf8(), a4 = supernode.toUtf8(), a5 = myIp.toUtf8();
        int rc = m_vlanStart(a1.constData(), a2.constData(), a3.constData(),
                             a4.constData(), a5.constData(), &core_log_cb);
        m_vlanRunning = (rc == 0);
        emit runningChanged();
        return rc;
    }
    Q_INVOKABLE int vlanStop() {
        if (!m_vlanStop) return -1;
        int rc = m_vlanStop();
        m_vlanRunning = false;
        emit runningChanged();
        return rc;
    }
    Q_INVOKABLE void copyToClipboard(const QString& text) {
        QGuiApplication::clipboard()->setText(text);
    }

public slots:
    void onCoreLog(int level, const QString& msg) {
        QString tag = level == 0 ? "[INFO] " : level == 1 ? "[WARN] " : "[ERROR] ";
        emit coreLog(tag, msg);
    }

signals:
    void tokenChanged();
    void runningChanged();
    void coreLog(const QString& level, const QString& line);
    void appError(const QString& msg);

private:
    void resolveFuncs() {
        m_frpStart = reinterpret_cast<FnFrpStart>(m_core.resolve("jdyc_frp_start"));
        m_frpStop  = reinterpret_cast<FnStop>(m_core.resolve("jdyc_frp_stop"));
        m_vlanStart = reinterpret_cast<FnVlanStart>(m_core.resolve("jdyc_vlan_start"));
        m_vlanStop = reinterpret_cast<FnStop>(m_core.resolve("jdyc_vlan_stop"));
        m_tapInstalled = reinterpret_cast<FnTapInstalled>(m_core.resolve("jdyc_tap_installed"));
        m_tapInstall = reinterpret_cast<FnTapInstall>(m_core.resolve("jdyc_tap_install"));
    }
    QString appDir() const {
        return QCoreApplication::applicationDirPath();
    }
    void writeTempFile(const QString& name, const QString& content) {
        QFile f(QDir::tempPath() + "/" + name);
        if (f.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
            f.write(content.toUtf8());
        }
    }
    typedef int (*FnFrpStart)(const char*, const char*, jdyc_log_cb);
    typedef int (*FnVlanStart)(const char*, const char*, const char*, const char*, const char*, jdyc_log_cb);
    typedef int (*FnStop)(void);
    typedef int (*FnTapInstalled)(void);
    typedef int (*FnTapInstall)(const char*);

    QSettings* m_settings = nullptr;
    QLibrary m_core;
    QString m_token;
    FnFrpStart m_frpStart = nullptr;
    FnStop m_frpStop = nullptr;
    FnVlanStart m_vlanStart = nullptr;
    FnStop m_vlanStop = nullptr;
    FnTapInstalled m_tapInstalled = nullptr;
    FnTapInstall m_tapInstall = nullptr;
    bool m_frpRunning = false;
    bool m_vlanRunning = false;
};

#include "main.moc"

// 引导 QML：窗口骨架 + WebSocket + 页面路由
static const char kBootQml[] = R"BOOTQML(
import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15
import QtWebSockets 1.15

ApplicationWindow {
    id: root
    visible: true
    width: 900; height: 620
    minimumWidth: 760; minimumHeight: 520
    title: "揭东一初联机大厅"
    color: "#f5f6f8"

    property string wsToken: ""
    property var roomData: null   // room_joined 全量字段

    // REST 助手
    function api(method, path, body, cb) {
        var xhr = new XMLHttpRequest();
        xhr.open(method, controller.baseUrl + path);
        xhr.setRequestHeader("Content-Type", "application/json");
        if (wsToken) xhr.setRequestHeader("Authorization", "Bearer " + wsToken);
        xhr.onreadystatechange = function() {
            if (xhr.readyState === XMLHttpRequest.DONE) {
                var data = {};
                try { data = JSON.parse(xhr.responseText); } catch (e) {}
                if (cb) cb(xhr.status, data);
            }
        };
        xhr.send(body ? JSON.stringify(body) : null);
    }

    function connectLobby() {
        wsToken = controller.token;
        ws.active = true;
    }

    function showJoined(d) {
        roomData = d;
        stack.push("qrc:/qml/RoomPage.qml");
    }

    function leaveRoom() {
        if (wsToken && roomData)
            ws.sendTextMessage(JSON.stringify({type: "leave_room", room_id: roomData.room_id}));
        controller.frpStop();
        controller.vlanStop();
        roomData = null;
        while (stack.depth > 1) stack.pop();
    }

    WebSocket {
        id: ws
        url: controller.baseUrl.replace(/^http/, "ws") + "/lobby?token=" + root.wsToken
        onStatusChanged: {
            if (status === WebSocket.Open) {
                statusLabel.text = "已连接大厅";
                ws.sendTextMessage(JSON.stringify({type: "list_rooms"}));
            }
            else if (status === WebSocket.Connecting) statusLabel.text = "连接大厅中…";
            else if (status === WebSocket.Error) { statusLabel.text = "大厅连接出错"; errorBar.show("大厅连接出错，请检查服务端地址"); }
            else statusLabel.text = "未连接大厅";
        }
        onTextMessageReceived: function(message) {
            var m = JSON.parse(message);
            switch (m.type) {
            case "room_list":
                lobbyModel.clear();
                for (var i = 0; i < m.rooms.length; i++) lobbyModel.append(m.rooms[i]);
                break;
            case "room_joined":
                root.showJoined(m);
                break;
            case "error":
                errorBar.show(m.msg || m.code || "请求失败");
                break;
            default: break; // room_created/room_removed/room_full/member_left 由 room_list 增量刷新
            }
        }
    }

    ListModel { id: lobbyModel }

    header: ToolBar {
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 16; anchors.rightMargin: 16
            Label { text: "揭东一初联机大厅"; font.pixelSize: 18; font.bold: true; color: "#2E5AAC" }
            Item { Layout.fillWidth: true }
            Label { id: statusLabel; text: "未连接大厅"; color: "#666666" }
            Button {
                text: "退出登录"
                flat: true
                onClicked: {
                    ws.active = false;
                    controller.token = "";
                    while (stack.depth > 0) stack.pop();
                    stack.push("qrc:/qml/LoginPage.qml");
                }
            }
        }
    }

    StackView {
        id: stack
        anchors.fill: parent
        initialItem: controller.token ? "qrc:/qml/LobbyPage.qml" : "qrc:/qml/LoginPage.qml"
    }

    Label {
        id: errorBar
        function show(msg) { text = msg; visible = true; hideTimer.restart(); }
        anchors.bottom: parent.bottom
        anchors.horizontalCenter: parent.horizontalCenter
        padding: 10
        color: "white"
        background: Rectangle { color: "#c0392b"; radius: 4 }
        visible: false
        Timer { id: hideTimer; interval: 4000; onTriggered: errorBar.visible = false }
    }
}
)BOOTQML";

int main(int argc, char* argv[])
{
    QCoreApplication::setAttribute(Qt::AA_EnableHighDpiScaling);
    QApplication app(argc, argv);
    QApplication::setApplicationName("揭东一初联机大厅");
    QApplication::setOrganizationName("jdyc");

    AppController controller;
    g_controller = &controller;
    QObject::connect(&controller, &AppController::appError, [](const QString& m) {
        qWarning() << "[核心]" << m;
    });

    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("controller", &controller);
    engine.loadData(QByteArray(kBootQml), QUrl("qrc:/boot.qml"));
    if (engine.rootObjects().isEmpty()) return 1;
    return app.exec();
}
