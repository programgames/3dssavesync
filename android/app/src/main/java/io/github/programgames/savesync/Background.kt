package io.github.programgames.savesync

import android.content.Context
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import java.util.concurrent.TimeUnit

// Background upload (off by default): upload only. Android can't see if an emulator runs, so a background Download
// could write under it; an Upload only reads the save (docs/guide-android.md).

// LastRun: when the last background pass ran, how many saves it uploaded, its problems (one per line; empty: none).
data class LastRun(val time: Long, val uploaded: Int, val problems: String)

const val UPLOAD_WORK = "upload"

// scheduleUploads turns the periodic work on or off: every 15 minutes (the shortest WorkManager allows), with a network
// and a battery that is not low. KEEP: turning it on again does not move the next run.
fun scheduleUploads(context: Context, on: Boolean) {
    val wm = WorkManager.getInstance(context)
    if (!on) {
        wm.cancelUniqueWork(UPLOAD_WORK)
        return
    }
    val constraints = Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).setRequiresBatteryNotLow(true).build()
    val request = PeriodicWorkRequestBuilder<UploadWorker>(15, TimeUnit.MINUTES).setConstraints(constraints).build()
    wm.enqueueUniquePeriodicWork(UPLOAD_WORK, ExistingPeriodicWorkPolicy.KEEP, request)
}

class UploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        App.init(applicationContext)
        App.backgroundPass()
        return Result.success() // a failure is in the last run line: no retry before the next period
    }
}
