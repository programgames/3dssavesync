plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "io.github.programgames.savesync"
    compileSdk = 35
    buildToolsVersion = "35.0.0"

    defaultConfig {
        applicationId = "io.github.programgames.savesync"
        minSdk = 24
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    lint {
        // Newer AndroidX versions need compileSdk 36; this first version stays on 35 (docs/adr/0026-client-android.md).
        disable += "GradleDependency"
    }
    testOptions {
        // The tests run in android/app/ (Robolectric reads the resources from there): the shared fixtures of every
        // client are ../../tests/.
        // Robolectric: the Compose screens run on the JVM, with the resources of the app (no emulator needed).
        unitTests.isIncludeAndroidResources = true
    }
}

dependencies {
    implementation(platform("androidx.compose:compose-bom:2025.05.00"))
    implementation("androidx.compose.material3:material3")
    implementation("androidx.activity:activity-compose:1.10.1")
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.work:work-runtime-ktx:2.10.0") // the background upload (Background.kt)
    // "Scan the PC's QR code" (Settings): Google code scanner, on the phone, without the CAMERA permission.
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")

    testImplementation("junit:junit:4.13.2")
    // org.json is part of Android, but the JVM unit tests only see stubs of it.
    testImplementation("org.json:json:20240303")
    testImplementation("org.robolectric:robolectric:4.14.1")
    testImplementation(platform("androidx.compose:compose-bom:2025.05.00"))
    testImplementation("androidx.compose.ui:ui-test-junit4")
    testImplementation("androidx.work:work-testing:2.10.0")
    debugImplementation("androidx.compose.ui:ui-test-manifest") // the empty activity of createAndroidComposeRule
}
