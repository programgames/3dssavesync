# Google code scanner (ML Kit, "Scan the PC's QR code"). Its internal classes are reached through reflection and its
# component registry; with R8 full mode (default since AGP 9) the 3.11.0 release lost them and startScan() failed at
# once with a NullPointerException in com.google.android.gms.internal.mlkit_code_scanner, while the debug build worked.
-keep class com.google.mlkit.** { *; }
-keep class com.google.android.gms.internal.mlkit_code_scanner.** { *; }
-keep class com.google.firebase.components.** { *; }
