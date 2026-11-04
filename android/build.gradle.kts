plugins {
    // AGP 9 compiles Kotlin itself (built-in Kotlin); the Compose plugin sets the Kotlin version (KGP) of the build.
    id("com.android.application") version "9.5.0" apply false
    id("org.jetbrains.kotlin.plugin.compose") version "2.4.21" apply false
}
