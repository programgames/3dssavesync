package io.github.programgames.savesync

import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.work.Configuration
import androidx.work.ListenableWorker
import androidx.work.NetworkType
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.TestListenableWorkerBuilder
import androidx.work.testing.WorkManagerTestInitHelper
import io.github.programgames.savesync.core.Uploads
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf

// Background upload: the periodic work follows the setting, and the problems are notified once while they stay.
@RunWith(RobolectricTestRunner::class)
class BackgroundTest {
    private val ctx: Context get() = ApplicationProvider.getApplicationContext()

    @Before
    fun setUp() {
        WorkManagerTestInitHelper.initializeTestWorkManager(ctx, Configuration.Builder().setExecutor(SynchronousExecutor()).build())
        App.init(ctx)
    }

    private fun work() = WorkManager.getInstance(ctx).getWorkInfosForUniqueWork(UPLOAD_WORK).get()

    @Test
    fun enqueuedWhenOnCancelledWhenOff() {
        assertFalse(App.settings.background) // off by default
        assertTrue(work().isEmpty())
        App.setBackground(true)
        val w = work().single()
        assertEquals(WorkInfo.State.ENQUEUED, w.state)
        assertEquals(15 * 60_000L, w.periodicityInfo!!.repeatIntervalMillis)
        assertEquals(NetworkType.CONNECTED, w.constraints.requiredNetworkType)
        assertTrue(w.constraints.requiresBatteryNotLow())
        assertTrue(App.settings.background)
        App.setBackground(false)
        assertEquals(WorkInfo.State.CANCELLED, work().single().state)
        assertFalse(App.settings.background)
    }

    // Off (or no server): the worker reads nothing and records nothing.
    @Test
    fun workerDoesNothingWhenOff() = runBlocking {
        assertEquals(ListenableWorker.Result.success(), TestListenableWorkerBuilder<UploadWorker>(ctx).build().doWork())
        assertNull(App.lastRun)
        App.settings.background = true
        assertEquals(ListenableWorker.Result.success(), TestListenableWorkerBuilder<UploadWorker>(ctx).build().doWork())
        assertNull(App.lastRun)
    }

    @Test
    fun problemsNotifiedOnce() {
        val nm = ctx.getSystemService(NotificationManager::class.java)
        val shown = { shadowOf(nm).allNotifications.size }
        val conflict = Uploads(2, listOf("Recent"), listOf("Pokemon Emeraude"), emptyList())
        App.record(conflict, emptyList(), now = 1_000)
        assertEquals(1, shown())
        assertEquals(LastRun(1_000, 2, ctx.getString(R.string.bg_conflict, "Pokemon Emeraude")), App.lastRun)
        assertEquals(App.lastRun, App.settings.lastRun)
        nm.cancelAll()
        App.record(conflict, emptyList())
        assertEquals(0, shown()) // same problems: not notified again
        App.record(Uploads(0, emptyList(), emptyList(), emptyList()), emptyList())
        assertEquals(0, shown())
        assertEquals("", App.lastRun!!.problems)
        App.record(conflict, emptyList())
        assertEquals(1, shown()) // back after a run without problems: notified again
    }
}
