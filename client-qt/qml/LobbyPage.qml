// LobbyPage.qml：房间列表 + 筛选 + 建房
import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15

Page {
    background: Rectangle { color: "#f5f6f8" }

    Component.onCompleted: root.connectLobby()

    property string filterText: ""

    function gameName(g) {
        return {minecraft: "我的世界 Java 版", terraria: "泰拉瑞亚",
                cs16: "反恐精英 1.6", yuzu: " Switch 模拟器", custom: "其他"}[g] || g;
    }

    function createRoom() {
        var mode = modeBox.currentValue;
        var body = {
            name: roomName.text, game: gameBox.currentValue,
            mode: mode, max_members: maxMembers.value
        };
        if (mode === "frp") {
            body.local_port = parseInt(localPort.text) || 0;
            body.port_type = portTypeBox.currentValue;
        }
        root.api("POST", "/api/rooms", body, function(code, data) {
            if (code === 200 && data.room) {
                // 建房成功 → 加入自己的房间，服务端回 room_joined 后进房间页
                ws.sendTextMessage(JSON.stringify({type: "join_room", room_id: data.room.id}));
            } else {
                errorBar.show(data.msg || "建房失败");
            }
        });
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 12

        RowLayout {
            Layout.fillWidth: true
            spacing: 10
            TextField {
                id: searchBox
                Layout.preferredWidth: 220
                placeholderText: "筛选房间名 / 房主"
                onTextChanged: root.filterText = text
            }
            Item { Layout.fillWidth: true }
            Button { text: "刷新列表"; flat: true; onClicked: ws.sendTextMessage(JSON.stringify({type: "list_rooms"})) }
            Button {
                text: "创建房间"
                highlighted: true
                onClicked: createDialog.open()
            }
        }

        ListView {
            id: roomList
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: 8
            model: lobbyModel
            delegate: Rectangle {
                width: roomList.width
                height: 72
                radius: 8
                color: "#ffffff"
                visible: filterText === "" ||
                         name.toLowerCase().indexOf(filterText.toLowerCase()) >= 0 ||
                         host_nickname.indexOf(filterText) >= 0
                RowLayout {
                    anchors.fill: parent
                    anchors.margins: 14
                    spacing: 12
                    Rectangle {
                        width: 44; height: 44; radius: 22
                        color: mode === "vlan" ? "#2E5AAC" : "#3A8F6C"
                        Label {
                            anchors.centerIn: parent
                            text: mode === "vlan" ? "VL" : "FR"
                            color: "white"; font.bold: true
                        }
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 2
                        Label { text: name; font.pixelSize: 15; font.bold: true }
                        Label {
                            text: root.gameName(game) + " · 房主 " + host_nickname +
                                  (host_class ? "（" + host_class + "）" : "")
                            font.pixelSize: 12; color: "#888888"
                        }
                    }
                    Label {
                        text: (typeof frp_addr !== "undefined" && frp_addr) ? frp_addr : ""
                        font.pixelSize: 12; color: "#3A8F6C"
                        visible: mode === "frp"
                    }
                    Label { text: members + "/" + max_members + " 人"; color: "#666666" }
                    Button {
                        text: "加入"
                        onClicked: ws.sendTextMessage(JSON.stringify({type: "join_room", room_id: id}))
                    }
                }
            }
            Label {
                anchors.centerIn: parent
                visible: roomList.count === 0
                text: "还没有房间，点右上角「创建房间」开一局"
                color: "#aaaaaa"
            }
        }
    }

    // 建房对话框
    Dialog {
        id: createDialog
        title: "创建房间"
        modal: true
        anchors.centerIn: parent
        width: 420
        standardButtons: Dialog.Ok | Dialog.Cancel
        onAccepted: createRoom()

        ColumnLayout {
            width: parent.width
            spacing: 10
            TextField {
                id: roomName
                Layout.fillWidth: true
                placeholderText: "房间名"
            }
            ComboBox {
                id: gameBox
                Layout.fillWidth: true
                textRole: "label"
                valueRole: "value"
                model: [
                    {label: "我的世界 Java 版", value: "minecraft"},
                    {label: "泰拉瑞亚", value: "terraria"},
                    {label: "反恐精英 1.6", value: "cs16"},
                    {label: "Switch 模拟器联机", value: "yuzu"},
                    {label: "其他", value: "custom"}
                ]
            }
            RowLayout {
                Layout.fillWidth: true
                ComboBox {
                    id: modeBox
                    textRole: "label"
                    valueRole: "value"
                    model: [
                        {label: "虚拟局域网（局域网联机）", value: "vlan"},
                        {label: "端口转发（好友直连地址）", value: "frp"}
                    ]
                    onCurrentValueChanged: frpRow.visible = (modeBox.currentValue === "frp")
                }
                Label { text: "人数上限" }
                SpinBox { id: maxMembers; from: 2; to: 100; value: 20 }
            }
            RowLayout {
                id: frpRow
                visible: false
                Layout.fillWidth: true
                Label { text: "本机端口" }
                TextField {
                    id: localPort
                    Layout.preferredWidth: 110
                    text: "25565"
                }
                ComboBox {
                    id: portTypeBox
                    textRole: "label"
                    valueRole: "value"
                    model: [
                        {label: "TCP", value: "tcp"},
                        {label: "UDP", value: "udp"},
                        {label: "TCP+UDP", value: "both"}
                    ]
                }
            }
        }
    }
}
