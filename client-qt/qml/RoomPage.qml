// RoomPage.qml：房间内页——连接信息、frpc/edge 启动、日志、离开
import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15

Page {
    background: Rectangle { color: "#f5f6f8" }

    property var d: root.roomData
    property bool isVlan: d && d.mode === "vlan"

    Component.onCompleted: {
        if (isVlan) {
            // TAP 检测：-10 未装则提示安装（需管理员 UAC 确认）
            if (controller.tapInstalled() === -10) {
                tapAsk.open();
            } else {
                startVlan();
            }
        } else {
            startFrp();
        }
    }

    function startFrp() {
        // 生成 frpc 最小 TOML（frps 参数全部来自服务端下发）
        var f = d.frp;
        var toml = "serverAddr = \"" + f.frpc_server + "\"\n" +
                   "serverPort = " + f.frpc_port + "\n" +
                   "auth.token = \"" + f.frpc_token + "\"\n\n";
        function proxy(name, type) {
            return "[[proxies]]\nname = \"" + name + "\"\ntype = \"" + type +
                   "\"\nlocalPort = " + f.local_port +
                   "\nremotePort = " + f.remote_port + "\n\n";
        }
        if (f.port_type === "tcp" || f.port_type === "both") toml += proxy(f.proxy_name + "-tcp", "tcp");
        if (f.port_type === "udp" || f.port_type === "both") toml += proxy(f.proxy_name + "-udp", "udp");
        var rc = controller.frpStart(toml);
        if (rc !== 0) errorBar.show("frpc 启动失败（错误码 " + rc + "）");
    }

    function startVlan() {
        var v = d.vlan;
        var rc = controller.vlanStart(v.network_community, v.vlan_key, v.supernode, v.my_virt_ip);
        if (rc !== 0) errorBar.show("edge 启动失败（错误码 " + rc + "）");
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 12

        RowLayout {
            Layout.fillWidth: true
            Label { text: d ? d.room_name : ""; font.pixelSize: 20; font.bold: true }
            Label {
                text: isVlan ? "虚拟局域网" : "端口转发"
                color: isVlan ? "#2E5AAC" : "#3A8F6C"
                font.pixelSize: 13
            }
            Item { Layout.fillWidth: true }
            Button {
                text: "离开房间"
                onClicked: root.leaveRoom()
            }
        }

        // ---- 连接信息卡 ----
        Rectangle {
            Layout.fillWidth: true
            implicitHeight: infoCol.implicitHeight + 32
            radius: 8
            color: "#ffffff"

            ColumnLayout {
                id: infoCol
                anchors.fill: parent
                anchors.margins: 16
                spacing: 8

                Label { text: "连接信息"; font.bold: true; font.pixelSize: 15 }

                // frp：游戏中直接连接此地址
                RowLayout {
                    visible: !isVlan
                    Layout.fillWidth: true
                    Label { text: "游戏内直连地址："; color: "#666666" }
                    Label {
                        id: frpAddr
                        text: d && d.frp ? (d.frp.frpc_server + ":" + d.frp.remote_port) : ""
                        font.family: "Consolas"
                        color: "#2E5AAC"
                    }
                    Button {
                        text: "复制"
                        flat: true
                        onClicked: controller.copyToClipboard(frpAddr.text)
                    }
                    Label { text: "（发送给同房间同学，游戏内直接连这个地址）"; font.pixelSize: 12; color: "#aaaaaa" }
                }

                // vlan：本机虚拟 IP 与网络名
                GridLayout {
                    visible: isVlan
                    columns: 2
                    rowSpacing: 6
                    columnSpacing: 16
                    Label { text: "本机虚拟 IP："; color: "#666666" }
                    Label {
                        id: myIp
                        text: d && d.vlan ? d.vlan.my_virt_ip : ""
                        font.family: "Consolas"
                        Layout.fillWidth: true
                    }
                    Label { text: "房主虚拟 IP："; color: "#666666" }
                    Label {
                        text: d && d.vlan ? d.vlan.host_virt_ip : ""
                        font.family: "Consolas"
                        Layout.fillWidth: true
                    }
                    Label { text: "虚拟网络："; color: "#666666" }
                    Label {
                        text: d && d.vlan ? d.vlan.network_community : ""
                        font.family: "Consolas"
                        Layout.fillWidth: true
                    }
                    Label {
                        Layout.columnSpan: 2
                        text: "房主在游戏里选「对局域网开放」，其他人按虚拟 IP 直连"
                        font.pixelSize: 12
                        color: "#aaaaaa"
                    }
                }
            }
        }

        // ---- 日志区 ----
        Rectangle {
            Layout.fillWidth: true
            Layout.fillHeight: true
            radius: 8
            color: "#1e1f24"
            clip: true

            ListView {
                id: logView
                anchors.fill: parent
                anchors.margins: 12
                model: ListModel { id: logModel }
                delegate: Label {
                    width: logView.width
                    text: levelTag + line
                    color: levelTag === "[ERROR] " ? "#e74c3c" : levelTag === "[WARN] " ? "#f39c12" : "#b8bcc8"
                    font.family: "Consolas"
                    font.pixelSize: 12
                    wrapMode: Text.WrapAnywhere
                }
                onCountChanged: positionViewAtEnd()
            }
        }
    }

    // TAP 未安装询问框（Win7 专用版场景）
    Dialog {
        id: tapAsk
        title: "需要安装虚拟网卡驱动"
        modal: true
        anchors.centerIn: parent
        Label {
            width: 320
            wrapMode: Text.Wrap
            text: "首次使用虚拟局域网需要安装 TAP 网卡驱动。\n点击「安装」后请在弹出的 UAC 窗口选择「是」。"
        }
        standardButtons: Dialog.Ok | Dialog.Cancel
        onAccepted: {
            var rc = controller.tapInstall();
            if (rc === 0) startVlan();
            else errorBar.show("TAP 驱动安装失败（错误码 " + rc + "）");
        }
        onRejected: root.leaveRoom()
    }

    Connections {
        target: controller
        function onCoreLog(level, line) {
            logModel.append({levelTag: level, line: line});
        }
    }
}
