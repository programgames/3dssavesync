import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Release APK (CI on a v* tag, docs/adr/0026-client-android.md): one fixed key, so that each new APK installs over the
// old one (an uninstall deletes the backup copies), and a versionCode that only grows. Without these variables, the
// release APK is built unsigned and the version is "dev".
val env: Map<String, String> = System.getenv()

android {
    namespace = "io.github.programgames.savesync"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.programgames.savesync"
        minSdk = 24
        targetSdk = 36 // not 37 yet: target 37 needs the local network permission (docs/adr/0026-client-android.md)
        versionCode = env["ANDROID_VERSION_CODE"]?.toIntOrNull() ?: 1
        versionName = env["ANDROID_VERSION_NAME"]?.takeIf { it.isNotBlank() } ?: "dev"
    }

    signingConfigs {
        env["ANDROID_KEYSTORE_FILE"]?.takeIf { it.isNotBlank() }?.let { keystore ->
            create("release") {
                storeFile = file(keystore)
                storePassword = env["ANDROID_KEYSTORE_PASSWORD"]
                keyAlias = env["ANDROID_KEY_ALIAS"]
                keyPassword = env["ANDROID_KEY_PASSWORD"]
            }
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = true // R8; the libraries bring their own keep rules (WorkManager workers, ML Kit)
            isShrinkResources = true // the resources no code uses (the app reads every string through R.string)
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
            signingConfig = signingConfigs.findByName("release")
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    testOptions {
        // The tests run in android/app/ (Robolectric reads the resources from there): the shared fixtures of every
        // client are ../../tests/.
        // Robolectric: the Compose screens run on the JVM, with the resources of the app (no emulator needed).
        unitTests.isIncludeAndroidResources = true
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

// Robolectric runs Android 16 (SDK 36, src/test/resources/robolectric.properties) only on Java 21; the app itself
// still builds with Java 17. add-opens: Robolectric sets up the shared memory of Android 16 through it.
tasks.withType<Test>().configureEach {
    javaLauncher.set(javaToolchains.launcherFor { languageVersion.set(JavaLanguageVersion.of(21)) })
    jvmArgs("--add-opens=java.base/jdk.internal.access=ALL-UNNAMED")
}

// The newest versions that build with compileSdk 36: Compose BOM 2026.08+, core 1.19+ need compileSdk 37.
dependencies {
    implementation(platform("androidx.compose:compose-bom:2026.06.01"))
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-core") // Icons.Filled.*: material3 1.4 no longer brings it
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.core:core-ktx:1.18.0")
    implementation("androidx.work:work-runtime-ktx:2.11.2") // the background upload (Background.kt)
    // "Scan the PC's QR code" (Settings): Google code scanner, on the phone, without the CAMERA permission.
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")

    testImplementation("junit:junit:4.13.2")
    // org.json is part of Android, but the JVM unit tests only see stubs of it.
    testImplementation("org.json:json:20240303")
    testImplementation("org.robolectric:robolectric:4.17")
    testImplementation(platform("androidx.compose:compose-bom:2026.06.01"))
    testImplementation("androidx.compose.ui:ui-test-junit4")
    testImplementation("androidx.work:work-testing:2.11.2")
    debugImplementation("androidx.compose.ui:ui-test-manifest") // the empty activity of createAndroidComposeRule
}
