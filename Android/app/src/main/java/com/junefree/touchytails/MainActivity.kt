package com.junefree.touchytails

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import com.junefree.touchytails.ui.theme.TouchyTailsTheme
import android.Manifest
import android.bluetooth.BluetoothDevice
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.unit.dp
class MainActivity : ComponentActivity() {

    private lateinit var bleManager: BleManager

    private val devices = mutableStateListOf<BluetoothDevice>()

    private var scanning by mutableStateOf(false)

    private lateinit var oscManager: OscManager

    private val permissionLauncher =
        registerForActivityResult(
            ActivityResultContracts.RequestMultiplePermissions()
        ) { permissions ->

            val granted =
                permissions[Manifest.permission.BLUETOOTH_SCAN] == true &&
                        permissions[Manifest.permission.BLUETOOTH_CONNECT] == true

            if (granted) {
                startBleScan()
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        oscManager = OscManager(9001)

        oscManager.onMessage = { address, arguments ->

            runOnUiThread {
                println("OSC: $address $arguments")
            }
        }

        oscManager.start()

        bleManager = BleManager(this)

        bleManager.onDeviceFound = { device ->
            runOnUiThread {
                if (!devices.any { it.address == device.address }) {
                    devices.add(device)
                }
            }
        }

        bleManager.onScanFinished = {
            runOnUiThread {
                scanning = false
            }
        }

        setContent {
            MaterialTheme {

                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(24.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {

                    Text(
                        text = "VR Haptics",
                        style = MaterialTheme.typography.headlineMedium
                    )

                    Button(
                        onClick = {
                            if (scanning) {
                                bleManager.stopScan()
                                scanning = false
                            } else {
                                requestBlePermissionsAndScan()
                            }
                        }
                    ) {
                        Text(
                            if (scanning) {
                                "Stop Scan"
                            } else {
                                "Scan for TouchyTails"
                            }
                        )
                    }

                    Text(
                        text = "Found: ${devices.size}"
                    )

                    LazyColumn(
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        items(devices) { device ->

                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(8.dp)
                            ) {
                                Text(
                                    text = device.name ?: "Unknown"
                                )

                                Text(
                                    text = device.address
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    private fun requestBlePermissionsAndScan() {

        if (bleManager.hasPermissions()) {
            startBleScan()
            return
        }

        permissionLauncher.launch(
            arrayOf(
                Manifest.permission.BLUETOOTH_SCAN,
                Manifest.permission.BLUETOOTH_CONNECT
            )
        )
    }

    private fun startBleScan() {
        devices.clear()
        scanning = true
        bleManager.startScan()
    }

    override fun onDestroy() {
        oscManager.stop()
        bleManager.stopScan()
        super.onDestroy()
    }
}