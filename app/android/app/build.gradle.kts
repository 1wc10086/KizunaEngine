import java.util.Properties

val keystoreProperties = Properties()
val keystoreFile = rootProject.file("key.properties")
if (keystoreFile.exists()) {
    keystoreFile.inputStream().use { keystoreProperties.load(it) }
}

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.kizuna.engine"
    compileSdk = 36
    ndkVersion = "27.3.13750724"

    defaultConfig {
        applicationId = "com.kizuna.engine"
        minSdk = flutter.minSdkVersion
        targetSdk = 36
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    signingConfigs {
        create("release") {
            if (keystoreFile.exists()) {
                storeFile = file(keystoreProperties.getProperty("storeFile"))
                storePassword = keystoreProperties.getProperty("storePassword")
                keyAlias = keystoreProperties.getProperty("keyAlias")
                keyPassword = keystoreProperties.getProperty("keyPassword")
            }
        }
    }

    // CI 的构建时间几乎全花在 R8 上（全量 shrink + 资源压缩 + 每个 ABI 一遍），
    // 而 Actions 按 job 分钟计费，所以这里把混淆/压缩整体关掉。Flutter 的
    // FlutterPlugin 会在插件被 apply 时把 release 打开 minify（并读取 -Pshrink），
    // 本脚本在 plugins 之后求值，这里的赋值覆盖它；工作流还会额外传
    // -Pshrink=false 兜底，两条路径都关得住。
    //
    // profile 由 AGP 从 debug 派生，本来就不混淆；显式写一遍是为了将来 Flutter
    // 改动 build type 时行为不变。
    //
    // 代价：release 包体积变大、没有标识符混淆。真要出商店包时，在
    // build-android.yml.1 里去掉 --no-shrink / -Pshrink=false，并临时把下面两个
    // 开关改回 true 即可，其余流程不用动。
    buildTypes {
        getByName("release") {
            signingConfig = if (keystoreFile.exists()) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
            isMinifyEnabled = false
            isShrinkResources = false
        }
        getByName("profile") {
            signingConfig = if (keystoreFile.exists()) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
            isMinifyEnabled = false
            isShrinkResources = false
        }
    }

    // 引擎库/FFmpeg 等 .so 体积很大，压缩进 APK 能把包体从 ~100MB 降到 ~30MB。
    // extractNativeLibs=true 让安装时解压到 lib/，运行期仍按常规路径加载。
    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
}

flutter {
    source = "../.."
}

dependencies {
    // SAF 目录镜像需要 DocumentFile 来递归遍历 content:// 树。
    implementation("androidx.documentfile:documentfile:1.0.1")
}
