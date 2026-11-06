allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory = rootProject.layout.buildDirectory.dir("../../build").get()
rootProject.layout.buildDirectory.value(newBuildDir)

// 把所有 Android 子项目（含 fvp、video_player 这类插件）的 ndkVersion 钉到
// app 已声明的版本。
//
// 插件自带的 build.gradle 往往写死 NDK 27.0.12077973，而 runner/Gradle 缓存里
// 只有 app 用的 27.3.13750724。AGP 发现缺版本就现场下载 ~600MB 的 NDK，CI 上
// 断流时会抛 ZipException（"Archive is not a ZIP archive"）直接让整个构建失败，
// 一次构建还要多花好几分钟。统一版本后不再触发下载。
// 用反射调用 setNdkVersion 而不是 import com.android.build.gradle.BaseExtension，
// 是因为根工程的构建脚本不在 AGP 的 classpath 上，直接引用会编译失败。
val pinnedNdkVersion = "27.3.13750724"

fun Project.pinPluginNdk() {
    val android = extensions.findByName("android") ?: return
    val setter = android.javaClass.methods.firstOrNull {
        it.name == "setNdkVersion" && it.parameterTypes.size == 1
    }
    runCatching { setter?.invoke(android, pinnedNdkVersion) }
}

// 必须在 evaluationDependsOn(":app") 之前注册：那条语句会提前求值 :app，
// 之后再对已求值的工程调 afterEvaluate 会直接报错。
subprojects {
    if (state.executed) {
        pinPluginNdk()
    } else {
        afterEvaluate { pinPluginNdk() }
    }
}

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
