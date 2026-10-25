package com.junefree.touchytails

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.HandlerThread
import android.os.Handler
import android.os.IBinder
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.Locale
import java.util.concurrent.ConcurrentHashMap

/** Live state shared between the service and the UI. */
object Hub {
    val status = MutableStateFlow<Map<String, String>>(emptyMap())
    val lastOsc = MutableStateFlow("")
    val running = MutableStateFlow(false)

    @Volatile
    var service: TailService? = null
}

/**
 * Foreground service: keeps the OSC listener and BLE links alive while VRChat
 * is in the foreground (e.g. on a standalone headset).
 */
class TailService : Service() {

    companion object {
        const val OSC_PORT = 9001
        private const val CHANNEL = "touchytails"
        private const val NOTIFICATION_ID = 1
        private const val PREFIX = "/avatar/parameters/"

        fun start(context: Context) =
            ContextCompat.startForegroundService(context, Intent(context, TailService::class.java))

        fun stop(context: Context) = context.stopService(Intent(context, TailService::class.java))

        /** Same mapping as the desktop app: 0..1 -> 0.8..1.0 */
        fun mapValue(v: Float): Float = maxOf(0.8f, 0.8f + v * 0.2f)

        fun format(v: Float): String = String.format(Locale.US, "%.2f", v)
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private lateinit var bleThread: HandlerThread
    private lateinit var bleHandler: Handler
    private val links = ConcurrentHashMap<String, BleLink>()
    private val lastValue = ConcurrentHashMap<String, Float>()
    private val osc = OscManager(OSC_PORT)

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        DeviceRepo.init(this)
        startInForeground()

        bleThread = HandlerThread("ble").apply { start() }
        bleHandler = Handler(bleThread.looper)

        osc.onMessage = ::onOsc
        osc.start()

        // Follow the saved device list: start/stop links as devices are added, removed, toggled
        scope.launch {
            DeviceRepo.devices.collect { list -> syncLinks(list) }
        }

        Hub.service = this
        Hub.running.value = true
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

    override fun onDestroy() {
        Hub.running.value = false
        Hub.service = null
        scope.cancel()
        osc.shutdown()
        links.values.forEach { it.stop() }
        links.clear()
        bleThread.quitSafely()
        super.onDestroy()
    }

    /** Test vibration from the UI. */
    fun test(address: String) {
        links[address]?.send(format(mapValue(Math.random().toFloat())))
    }

    private fun syncLinks(list: List<DeviceConfig>) {
        val wanted = list.filter { it.enabled }.map { it.address }.toSet()
        (links.keys - wanted).forEach { addr ->
            links.remove(addr)?.stop()
            lastValue.remove(addr)
        }
        list.forEach { cfg ->
            if (!cfg.enabled) setStatus(cfg.address, "Disabled")
        }
        (wanted - links.keys).forEach { addr ->
            val link = BleLink(this, addr, bleHandler, ::setStatus)
            links[addr] = link
            link.start()
        }
        Hub.status.update { m -> m.filterKeys { k -> list.any { it.address == k } } }
    }

    private fun setStatus(address: String, status: String) {
        Hub.status.update { it + (address to status) }
    }

    /** Called on the OSC thread. */
    private fun onOsc(address: String, args: List<Any?>) {
        if (!address.startsWith(PREFIX)) return
        val name = address.removePrefix(PREFIX)
        val v = when (val a = args.firstOrNull()) {
            is Float -> a
            is Double -> a.toFloat()
            is Int -> a.toFloat()
            is Boolean -> if (a) 1f else 0f
            else -> return
        }

        for (cfg in DeviceRepo.devices.value) {
            if (!cfg.enabled || cfg.param != name) continue
            val link = links[cfg.address] ?: continue
            val prev = lastValue.put(cfg.address, v) ?: 0f
            if (v == 0f && prev == 0f) continue
            val out = format(mapValue(v))
            link.send(out)
            Hub.lastOsc.value = "${cfg.name}: $name = ${format(v)} -> $out"
        }
    }

    private fun startInForeground() {
        val nm = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            nm.createNotificationChannel(
                NotificationChannel(CHANNEL, "TouchyTails", NotificationManager.IMPORTANCE_LOW)
            )
        }
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val notification: Notification = androidx.core.app.NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(android.R.drawable.stat_sys_data_bluetooth)
            .setContentTitle("TouchyTails")
            .setContentText("Listening for VRChat OSC on UDP $OSC_PORT")
            .setContentIntent(open)
            .setOngoing(true)
            .build()

        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q)
            ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE else 0
        ServiceCompat.startForeground(this, NOTIFICATION_ID, notification, type)
    }
}
