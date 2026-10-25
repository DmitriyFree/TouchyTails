package com.junefree.touchytails

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import org.json.JSONArray
import org.json.JSONObject

/** Persistent device settings, same fields as devices.json of the desktop app. */
data class DeviceConfig(
    val address: String,
    val name: String,
    val enabled: Boolean = true,
    val param: String = "",
)

/** Single source of truth for saved devices. UI edits it, the service follows it. */
object DeviceRepo {
    private const val PREFS = "devices"
    private const val KEY = "list"

    private lateinit var prefs: SharedPreferences
    private val _devices = MutableStateFlow<List<DeviceConfig>>(emptyList())
    val devices: StateFlow<List<DeviceConfig>> = _devices

    fun init(context: Context) {
        if (::prefs.isInitialized) return
        prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        _devices.value = load()
    }

    fun add(address: String) {
        _devices.update { list ->
            if (list.any { it.address == address }) list
            else list + DeviceConfig(address, "Device ${nextLetter(list)}")
        }
        save()
    }

    fun update(address: String, change: (DeviceConfig) -> DeviceConfig) {
        _devices.update { list -> list.map { if (it.address == address) change(it) else it } }
        save()
    }

    fun remove(address: String) {
        _devices.update { list -> list.filterNot { it.address == address } }
        save()
    }

    private fun nextLetter(list: List<DeviceConfig>): String =
        ('A'..'Z').map { it.toString() }.firstOrNull { l -> list.none { it.name == "Device $l" } }
            ?: (list.size + 1).toString()

    private fun load(): List<DeviceConfig> {
        val raw = prefs.getString(KEY, null) ?: return emptyList()
        return try {
            val arr = JSONArray(raw)
            (0 until arr.length()).map { i ->
                val o = arr.getJSONObject(i)
                DeviceConfig(
                    address = o.getString("id"),
                    name = o.optString("name", "Device"),
                    enabled = o.optBoolean("enabled", true),
                    param = o.optString("event", ""),
                )
            }
        } catch (e: Exception) {
            emptyList()
        }
    }

    private fun save() {
        val arr = JSONArray()
        _devices.value.forEach {
            arr.put(
                JSONObject()
                    .put("id", it.address)
                    .put("name", it.name)
                    .put("enabled", it.enabled)
                    .put("event", it.param)
            )
        }
        prefs.edit().putString(KEY, arr.toString()).apply()
    }
}
