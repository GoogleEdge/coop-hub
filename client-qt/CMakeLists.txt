# 揭东一初联机大厅 Win7 专用版（Qt 5.15 + MSVC 2019 x64）
# 由 CI 传入：-DCMAKE_BUILD_TYPE=Release -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded
cmake_minimum_required(VERSION 3.16)
project(JDYCHub VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt5 REQUIRED COMPONENTS Quick QuickControls2 WebSockets)

add_executable(JDYCHub WIN32
  main.cpp
  app.rc
)

target_link_libraries(JDYCHub PRIVATE
  Qt5::Quick
  Qt5::QuickControls2
  Qt5::WebSockets
)

# QML 页面与图标打进可执行文件资源
qt5_add_resources(JDYCHub "app_res"
  PREFIX "/"
  FILES
    qml/LoginPage.qml
    qml/LobbyPage.qml
    qml/RoomPage.qml
)

set_target_properties(JDYCHub PROPERTIES
  OUTPUT_NAME "JDYCHub"
)
