package com.junefree.touchytails

import android.util.Log
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetSocketAddress
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.Executors
import java.util.concurrent.Future

class OscManager(
    private val port: Int = 9001
) {

    companion object {
        private const val TAG = "OscManager"
        private const val BUFFER_SIZE = 65535
    }

    private val executor = Executors.newSingleThreadExecutor()

    private var socket: DatagramSocket? = null
    private var listenerTask: Future<*>? = null

    @Volatile
    private var running = false

    var onMessage: ((address: String, arguments: List<Any?>) -> Unit)? = null

    fun start() {
        if (running) {
            Log.d(TAG, "OSC already running")
            return
        }

        running = true

        listenerTask = executor.submit {
            listen()
        }
    }

    private fun listen() {
        try {
            val newSocket = DatagramSocket(null)

            newSocket.reuseAddress = true

            newSocket.bind(
                InetSocketAddress(
                    "0.0.0.0",
                    port
                )
            )

            socket = newSocket

            Log.i(
                TAG,
                "OSC listening on UDP $port"
            )

            val buffer = ByteArray(BUFFER_SIZE)

            while (running) {

                val packet = DatagramPacket(
                    buffer,
                    buffer.size
                )

                try {
                    newSocket.receive(packet)
                } catch (e: Exception) {
                    if (running) {
                        Log.e(
                            TAG,
                            "Error receiving OSC packet",
                            e
                        )
                    }

                    break
                }

                if (!running) {
                    break
                }

                try {
                    val data = packet.data.copyOf(
                        packet.length
                    )

                    val message = parseOscMessage(data)

                    if (message != null) {

                        Log.d(
                            TAG,
                            "OSC: ${message.address} ${message.arguments}"
                        )

                        onMessage?.invoke(
                            message.address,
                            message.arguments
                        )
                    }

                } catch (e: Exception) {
                    Log.e(
                        TAG,
                        "Failed to parse OSC packet",
                        e
                    )
                }
            }

        } catch (e: Exception) {

            if (running) {
                Log.e(
                    TAG,
                    "Failed to start OSC listener",
                    e
                )
            }

        } finally {

            socket?.close()
            socket = null

            Log.i(
                TAG,
                "OSC listener stopped"
            )
        }
    }

    fun stop() {
        if (!running) {
            return
        }

        Log.i(
            TAG,
            "Stopping OSC listener"
        )

        running = false

        /*
         * Closing the socket causes receive()
         * to immediately return/throw, allowing
         * the listener thread to exit.
         */
        socket?.close()
        socket = null

        listenerTask?.cancel(true)
        listenerTask = null
    }

    fun isRunning(): Boolean {
        return running
    }

    fun shutdown() {
        stop()
        executor.shutdownNow()
    }

    private fun parseOscMessage(
        data: ByteArray
    ): OscMessage? {

        val buffer = ByteBuffer
            .wrap(data)
            .order(ByteOrder.BIG_ENDIAN)

        val address = readOscString(buffer)
            ?: return null

        val typeTags = readOscString(buffer)
            ?: return null

        if (!typeTags.startsWith(",")) {
            return null
        }

        val arguments = mutableListOf<Any?>()

        for (type in typeTags.substring(1)) {

            when (type) {

                'f' -> {
                    arguments.add(
                        buffer.float
                    )
                }

                'i' -> {
                    arguments.add(
                        buffer.int
                    )
                }

                'h' -> {
                    arguments.add(
                        buffer.long
                    )
                }

                'd' -> {
                    arguments.add(
                        buffer.double
                    )
                }

                's' -> {
                    arguments.add(
                        readOscString(buffer)
                    )
                }

                'b' -> {
                    arguments.add(
                        readOscBlob(buffer)
                    )
                }

                'T' -> {
                    arguments.add(true)
                }

                'F' -> {
                    arguments.add(false)
                }

                'N' -> {
                    arguments.add(null)
                }

                'I' -> {
                    arguments.add(
                        "Impulse"
                    )
                }

                else -> {
                    Log.w(
                        TAG,
                        "Unsupported OSC type: $type"
                    )

                    return null
                }
            }
        }

        return OscMessage(
            address = address,
            arguments = arguments
        )
    }

    private fun readOscString(
        buffer: ByteBuffer
    ): String? {

        if (!buffer.hasRemaining()) {
            return null
        }

        val bytes = mutableListOf<Byte>()

        while (buffer.hasRemaining()) {

            val byte = buffer.get()

            if (byte.toInt() == 0) {
                break
            }

            bytes.add(byte)
        }

        /*
         * OSC strings are padded to a multiple
         * of four bytes.
         */
        val consumed = bytes.size + 1
        val padding = (4 - (consumed % 4)) % 4

        if (buffer.remaining() < padding) {
            return null
        }

        buffer.position(
            buffer.position() + padding
        )

        return bytes.toByteArray()
            .toString(Charsets.UTF_8)
    }

    private fun readOscBlob(
        buffer: ByteBuffer
    ): ByteArray? {

        if (buffer.remaining() < 4) {
            return null
        }

        val size = buffer.int

        if (size < 0 || buffer.remaining() < size) {
            return null
        }

        val data = ByteArray(size)

        buffer.get(data)

        val padding = (4 - (size % 4)) % 4

        if (buffer.remaining() < padding) {
            return null
        }

        buffer.position(
            buffer.position() + padding
        )

        return data
    }
}

data class OscMessage(
    val address: String,
    val arguments: List<Any?>
)