// LoginPage.qml：登录 / 注册（含 30 项班级下拉）
import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15

Page {
    background: Rectangle { color: "#f5f6f8" }

    Component.onCompleted: {
        if (controller.token.length > 0) root.connectLobby();
    }

    // 30 个班级：初一1~10班 / 初二1~10班 / 初三1~10班
    readonly property var classNames: {
        var arr = [];
        var grades = ["初一", "初二", "初三"];
        for (var g = 0; g < grades.length; g++)
            for (var i = 1; i <= 10; i++)
                arr.push(grades[g] + i + "班");
        return arr;
    }

    Flickable {
        anchors.fill: parent
        contentHeight: form.implicitHeight + 64
        clip: true

        ColumnLayout {
            id: form
            width: Math.min(420, parent.width - 48)
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.topMargin: 32
            spacing: 12

            Label {
                text: "揭东一初联机大厅"
                font.pixelSize: 26; font.bold: true
                color: "#2E5AAC"
                Layout.alignment: Qt.AlignHCenter
            }
            Label {
                text: "实名审核后即可联机"
                font.pixelSize: 13; color: "#888888"
                Layout.alignment: Qt.AlignHCenter
            }

            TabBar {
                id: tabs
                Layout.fillWidth: true
                TabButton { text: "登录" }
                TabButton { text: "注册" }
            }

            StackLayout {
                Layout.fillWidth: true
                currentIndex: tabs.currentIndex

                // ---- 登录 ----
                ColumnLayout {
                    spacing: 12
                    TextField {
                        id: loginUser
                        Layout.fillWidth: true
                        placeholderText: "用户名"
                    }
                    TextField {
                        id: loginPass
                        Layout.fillWidth: true
                        placeholderText: "密码"
                        echoMode: TextInput.Password
                    }
                    Button {
                        Layout.fillWidth: true
                        text: "登 录"
                        highlighted: true
                        onClicked: {
                            root.api("POST", "/api/login",
                                     {username: loginUser.text, password: loginPass.text},
                                     function(code, data) {
                                if (code === 200 && data.token) {
                                    controller.token = data.token;
                                    root.connectLobby();
                                    stack.pop();
                                    stack.push("qrc:/qml/LobbyPage.qml");
                                } else {
                                    errorBar.show(data.msg || "登录失败");
                                }
                            });
                        }
                    }
                }

                // ---- 注册 ----
                ColumnLayout {
                    spacing: 10
                    TextField {
                        id: regUser; Layout.fillWidth: true
                        placeholderText: "用户名（4-20 位字母数字下划线）"
                    }
                    TextField {
                        id: regNick; Layout.fillWidth: true
                        placeholderText: "昵称"
                    }
                    TextField {
                        id: regPass; Layout.fillWidth: true
                        placeholderText: "密码（8-64 位，含字母和数字）"
                        echoMode: TextInput.Password
                    }
                    ComboBox {
                        id: regClass
                        Layout.fillWidth: true
                        model: root.classNames
                    }
                    TextField {
                        id: regRealName; Layout.fillWidth: true
                        placeholderText: "真实姓名（2-4 个汉字，用于审核）"
                    }
                    TextField {
                        id: regStudentNo; Layout.fillWidth: true
                        placeholderText: "学号（选填）"
                    }
                    Label {
                        text: "注册后需管理员实名审核通过才能进入大厅"
                        font.pixelSize: 12; color: "#888888"
                    }
                    Button {
                        Layout.fillWidth: true
                        text: "注 册"
                        highlighted: true
                        onClicked: {
                            root.api("POST", "/api/register", {
                                username: regUser.text, password: regPass.text,
                                nickname: regNick.text,
                                class_name: regClass.currentText,
                                real_name: regRealName.text,
                                student_no: regStudentNo.text
                            }, function(code, data) {
                                if (code === 201) {
                                    tabs.currentIndex = 0;
                                    loginUser.text = regUser.text;
                                    errorBar.show("注册成功，等待审核");
                                } else {
                                    errorBar.show(data.msg || "注册失败");
                                }
                            });
                        }
                    }
                }
            }
            Item { Layout.fillHeight: true }
        }
    }
}
