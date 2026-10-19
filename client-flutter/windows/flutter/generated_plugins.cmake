# 插件构建规则（flutter 工具在 pub get 时自动重写此文件）
# 手写空版本：本应用不依赖带平台通道的插件，shared_preferences 的 windows
# 实现会在 pub get 后由工具写入真实列表。
list(APPEND FLUTTER_PLUGIN_LIST
)

list(APPEND FLUTTER_FFI_PLUGIN_LIST
)

set(PLUGIN_BUNDLED_LIBRARIES)

foreach(plugin ${FLUTTER_PLUGIN_LIST})
  add_subdirectory(flutter/ephemeral/.plugin_symlinks/${plugin}/windows plugins/${plugin})
  target_link_libraries(${BINARY_NAME} PRIVATE ${plugin}_plugin)
  list(APPEND PLUGIN_BUNDLED_LIBRARIES $<TARGET_FILE:${plugin}_plugin>)
endforeach(plugin)

foreach(ffi_plugin ${FLUTTER_FFI_PLUGIN_LIST})
  add_subdirectory(flutter/ephemeral/.plugin_symlinks/${ffi_plugin}/windows plugins/${ffi_plugin})
  list(APPEND PLUGIN_BUNDLED_LIBRARIES ${ffi_plugin}_bundled_libraries)
endforeach(ffi_plugin)
