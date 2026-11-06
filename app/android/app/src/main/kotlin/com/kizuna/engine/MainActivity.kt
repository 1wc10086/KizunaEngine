package com.kizuna.engine

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.DocumentsContract
import android.provider.Settings
import androidx.core.content.FileProvider
import androidx.documentfile.provider.DocumentFile
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File
import java.io.FileOutputStream

class MainActivity : FlutterActivity() {
    companion object {
        private const val FOLDER_CHANNEL = "kizuna/folder"
        private const val REQUEST_PICK_FOLDER = 4711
        private const val REQUEST_ALL_FILES = 4712
    }

    private var pendingFolderResult: MethodChannel.Result? = null

    // “所有文件访问权”在 Android 11+ 只能跳系统设置授予；这里保存等待回包的
    // MethodChannel.Result，用户从设置页返回后再按实际状态回给 Dart。
    private var pendingStorageResult: MethodChannel.Result? = null

    override fun configureFlutterEngine(engine: FlutterEngine) {
        super.configureFlutterEngine(engine)
        MethodChannel(engine.dartExecutor.binaryMessenger, "kizuna/installer").setMethodCallHandler { call, result ->
            when (call.method) {
                "installApk" -> {
                    val path = call.argument<String>("path")
                    if (path == null) {
                        result.error("BAD_ARGS", "Missing path", null)
                        return@setMethodCallHandler
                    }
                    installApk(File(path), result)
                }
                "abi" -> result.success(abiTag())
                "hasAllFilesAccess" -> result.success(hasAllFilesAccess())
                "requestAllFilesAccess" -> requestAllFilesAccess(result)
                else -> result.notImplemented()
            }
        }
        MethodChannel(engine.dartExecutor.binaryMessenger, FOLDER_CHANNEL).setMethodCallHandler { call, result ->
            when (call.method) {
                "pickFolder" -> pickFolder(result)
                "mirrorFolder" -> {
                    val uri = call.argument<String>("uri")
                    val name = call.argument<String>("name") ?: "game"
                    if (uri == null) {
                        result.error("BAD_ARGS", "Missing uri", null)
                    } else {
                        mirrorOnBackground(Uri.parse(uri), name, result)
                    }
                }
                else -> result.notImplemented()
            }
        }
    }

    /**
     * 当前是否拥有“管理所有文件”的能力。
     *
     * Android 11+ 的 MANAGE_EXTERNAL_STORAGE 是特殊权限，不能用
     * requestPermissions 申请，只能查 Environment.isExternalStorageManager()
     * 并引导用户去系统设置。Android 10 及以下仍然走普通存储运行时权限，
     * 读写都要有——requestLegacyExternalStorage 只关闭分区存储，不放宽授权。
     */
    private fun hasAllFilesAccess(): Boolean {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            Environment.isExternalStorageManager()
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            checkSelfPermission(Manifest.permission.READ_EXTERNAL_STORAGE) ==
                PackageManager.PERMISSION_GRANTED &&
                checkSelfPermission(Manifest.permission.WRITE_EXTERNAL_STORAGE) ==
                PackageManager.PERMISSION_GRANTED
        } else {
            true
        }
    }

    private fun requestAllFilesAccess(result: MethodChannel.Result) {
        if (hasAllFilesAccess()) {
            result.success(true)
            return
        }
        if (pendingStorageResult != null) {
            result.error("BUSY", "Another storage permission request is already open", null)
            return
        }
        pendingStorageResult = result
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            // 优先打开本应用的专属开关；部分 OEM 没有这个 Activity 时退回总列表。
            val appIntent = Intent(
                Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION,
                Uri.parse("package:$packageName"),
            )
            try {
                startActivityForResult(appIntent, REQUEST_ALL_FILES)
            } catch (_: Exception) {
                try {
                    startActivityForResult(
                        Intent(Settings.ACTION_MANAGE_ALL_FILES_ACCESS_PERMISSION),
                        REQUEST_ALL_FILES,
                    )
                } catch (error: Exception) {
                    pendingStorageResult = null
                    result.error("UNAVAILABLE", error.message ?: error.toString(), null)
                }
            }
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            requestPermissions(
                arrayOf(
                    Manifest.permission.READ_EXTERNAL_STORAGE,
                    Manifest.permission.WRITE_EXTERNAL_STORAGE,
                ),
                REQUEST_ALL_FILES,
            )
        } else {
            // Android 6 以下安装即授予；理论上 hasAllFilesAccess 已经提前返回，
            // 这里保留兜底并让 lint 的 NewApi 检查看到明确的版本分支。
            pendingStorageResult = null
            result.success(true)
        }
    }

    private fun installApk(file: File, result: MethodChannel.Result) {
        try {
            if (!file.exists()) {
                result.error("BAD_ARGS", "APK file not found: ${file.absolutePath}", null)
                return
            }
            val uri: Uri = FileProvider.getUriForFile(this, "$packageName.fileProvider", file)
            val intent = Intent(Intent.ACTION_VIEW)
            intent.setDataAndType(uri, "application/vnd.android.package-archive")
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            startActivity(intent)
            result.success(null)
        } catch (e: Exception) {
            result.error("INSTALL_FAILED", e.message ?: e.toString(), null)
        }
    }

    private fun abiTag(): String {
        for (abi in Build.SUPPORTED_ABIS) {
            when {
                abi == "arm64-v8a" -> return "arm64"
                abi.startsWith("armeabi") -> return "arm32"
                abi == "x86_64" || abi == "x86" -> return "x64"
            }
        }
        return "arm64"
    }

    /**
     * 用 SAF 选择目录。返回 {path, uri}：
     * - path 能直接映射到 /storage/... 时，Go 引擎可以直接读写；
     * - 映射不了（Downloads/云盘等）时只返回 uri，Dart 侧再调用 mirrorFolder
     *   把内容复制到应用私有目录。
     */
    private fun pickFolder(result: MethodChannel.Result) {
        if (pendingFolderResult != null) {
            result.error("BUSY", "Another folder picker is already open", null)
            return
        }
        pendingFolderResult = result
        try {
            val intent = Intent(Intent.ACTION_OPEN_DOCUMENT_TREE).apply {
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
                addFlags(Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION)
                addFlags(Intent.FLAG_GRANT_PREFIX_URI_PERMISSION)
            }
            startActivityForResult(intent, REQUEST_PICK_FOLDER)
        } catch (e: Exception) {
            pendingFolderResult = null
            result.error("PICK_FAILED", e.message ?: e.toString(), null)
        }
    }

    @Deprecated("FlutterActivity 仍是 Activity，用兼容回调避免引入额外依赖")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQUEST_ALL_FILES) {
            // 系统设置页不返回授权结果，只能重新查一次；用户没开也会如实回 false。
            val result = pendingStorageResult
            pendingStorageResult = null
            result?.success(hasAllFilesAccess())
            return
        }
        if (requestCode != REQUEST_PICK_FOLDER) {
            return
        }
        val result = pendingFolderResult
        pendingFolderResult = null
        if (result == null) {
            return
        }
        val uri = data?.data
        if (resultCode != Activity.RESULT_OK || uri == null) {
            result.success(null)
            return
        }
        try {
            contentResolver.takePersistableUriPermission(
                uri,
                Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION,
            )
        } catch (_: SecurityException) {
            // 某些 provider 不允许持久授权；mirror 仍然可用。
        } catch (_: Exception) {
        }
        result.success(
            mapOf(
                "path" to resolveTreePath(uri),
                "uri" to uri.toString(),
            ),
        )
    }

    /** Android 10 及以下的 READ/WRITE_EXTERNAL_STORAGE 走普通运行时权限回调。 */
    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_ALL_FILES) {
            return
        }
        val result = pendingStorageResult
        pendingStorageResult = null
        result?.success(hasAllFilesAccess())
    }

    /** 把 SAF 的 tree document id 还原成公共存储路径；无法还原时返回 null。 */
    private fun resolveTreePath(uri: Uri): String? {
        return try {
            val documentId = DocumentsContract.getTreeDocumentId(uri)
            val parts = documentId.split(":", limit = 2)
            val relative = if (parts.size > 1) parts[1] else ""
            val base = when (parts[0]) {
                "primary" -> Environment.getExternalStorageDirectory()
                "raw" -> File("/")
                else -> null
            } ?: return null
            val file = if (relative.isEmpty()) base else File(base, relative)
            if (file.isDirectory && file.canRead()) file.absolutePath else null
        } catch (_: Exception) {
            null
        }
    }

    /**
     * SAF 镜像：没有所有文件访问权限时，把选中的目录递归复制到应用私有目录。
     * 这是“只读游玩”的兜底方案；存档也会写进镜像目录，不会回写原目录。
     */
    private fun mirrorOnBackground(uri: Uri, name: String, result: MethodChannel.Result) {
        Thread {
            try {
                val key = Integer.toHexString(uri.toString().hashCode())
                val root = File(filesDir, "saf_mirror/$key")
                val ready = File(root, ".ready")
                if (ready.isFile) {
                    runOnUiThread { result.success(root.absolutePath) }
                } else {
                    val temporary = File(filesDir, "saf_mirror/$key.tmp")
                    temporary.deleteRecursively()
                    temporary.mkdirs()
                    val tree = DocumentFile.fromTreeUri(this, uri)
                        ?: throw IllegalStateException("Cannot open SAF tree")
                    copyDocument(tree, temporary)
                    File(temporary, ".ready").writeText("${uri}\n$name\n")
                    root.deleteRecursively()
                    if (!temporary.renameTo(root)) {
                        throw IllegalStateException("Cannot finalize SAF mirror")
                    }
                    runOnUiThread { result.success(root.absolutePath) }
                }
            } catch (e: Exception) {
                runOnUiThread {
                    result.error("MIRROR_FAILED", e.message ?: e.toString(), null)
                }
            }
        }.start()
    }

    private fun copyDocument(source: DocumentFile, destination: File) {
        if (source.isDirectory) {
            destination.mkdirs()
            for (child in source.listFiles()) {
                val childName = child.name ?: continue
                copyDocument(child, File(destination, sanitizeName(childName)))
            }
            return
        }
        if (!source.isFile) {
            return
        }
        val safeName = sanitizeName(source.name ?: "file")
        val target = File(destination, safeName)
        target.parentFile?.mkdirs()
        contentResolver.openInputStream(source.uri)?.use { input ->
            FileOutputStream(target).use { output ->
                input.copyTo(output, 1 shl 20)
            }
        }
    }

    private fun sanitizeName(name: String): String {
        val cleaned = name.replace(Regex("[\\\\/:*?\"<>|\\r\\n\\u0000]"), "_").trim()
        return if (cleaned.isEmpty() || cleaned == "." || cleaned == "..") "_" else cleaned
    }
}
