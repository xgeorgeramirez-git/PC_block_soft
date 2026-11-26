package ru.atlas404.pcblock.ui

import android.content.Context
import android.media.AudioAttributes
import android.media.MediaPlayer
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.LifecycleResumeEffect

/**
 * Прослушивание звука сообщения на телефоне (FR-41): mp3 из assets (sounds/message/N.mp3) через
 * MediaPlayer. Новое нажатие останавливает прежний звук и играет выбранный сначала; release() —
 * при уходе с экрана, чтобы звук не продолжался в фоне.
 */
class SoundPreview(private val context: Context) {
    private var player: MediaPlayer? = null

    fun play(n: Int) {
        stop()
        val p = MediaPlayer()
        try {
            context.assets.openFd(MessageAssets.soundPath(n)).use { fd ->
                p.setDataSource(fd.fileDescriptor, fd.startOffset, fd.length)
            }
            p.setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_MEDIA)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                    .build(),
            )
            p.setOnCompletionListener { done ->
                if (player === done) player = null
                done.release()
            }
            p.prepare() // файл в APK, короткий: подготовка быстрая
            p.start()
            player = p
        } catch (e: Exception) {
            p.release() // звук не прочитался: просто тишина, отправка не зависит от прослушивания
        }
    }

    fun stop() {
        val p = player ?: return
        player = null
        try {
            p.stop()
        } catch (e: IllegalStateException) {
            // уже остановлен
        }
        p.release()
    }
}

/** Плеер прослушивания на время экрана: уход с экрана (и уход приложения в фон) — тишина. */
@Composable
fun rememberSoundPreview(): SoundPreview {
    val context = LocalContext.current.applicationContext
    val preview = remember(context) { SoundPreview(context) }
    DisposableEffect(preview) {
        onDispose { preview.stop() }
    }
    LifecycleResumeEffect(preview) {
        onPauseOrDispose { preview.stop() }
    }
    return preview
}
