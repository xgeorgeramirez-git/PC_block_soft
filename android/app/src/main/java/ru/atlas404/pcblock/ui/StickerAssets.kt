package ru.atlas404.pcblock.ui

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import com.airbnb.lottie.compose.LottieAnimation
import com.airbnb.lottie.compose.LottieCompositionSpec
import com.airbnb.lottie.compose.LottieConstants
import com.airbnb.lottie.compose.rememberLottieComposition
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import ru.atlas404.pcblock.proto.Proto
import java.util.zip.GZIPInputStream

/**
 * Стикеры и звуки сообщения из assets приложения (папка assets репозитория, app/build.gradle.kts).
 * Набор — по файлам: stickers/NNN.tgs (превью — stickers/NNN.png, первый кадр 512 px) и
 * sounds/message/N.mp3. Нового файла достаточно, чтобы появился новый стикер или звук (D-101).
 */
object MessageAssets {
    private const val STICKERS = "stickers"
    const val SOUNDS = "sounds/message"

    @Volatile private var stickerList: List<String>? = null
    @Volatile private var soundList: List<Int>? = null

    /** Номера стикеров по возрастанию («001», «004», …). */
    fun stickers(context: Context): List<String> =
        stickerList ?: Proto.stickerIds(runCatching { context.assets.list(STICKERS) }.getOrNull()).also { stickerList = it }

    /** Номера звуков по возрастанию (1…5). */
    fun sounds(context: Context): List<Int> =
        soundList ?: Proto.soundIds(runCatching { context.assets.list(SOUNDS) }.getOrNull()).also { soundList = it }

    fun soundPath(n: Int): String = "$SOUNDS/$n.mp3"

    // Превью в памяти: до 1/8 памяти приложения, счёт в КБ (весь набор при 256 px — около 19 МБ).
    private val previews = object : LruCache<String, Bitmap>((Runtime.getRuntime().maxMemory() / 1024 / 8).toInt()) {
        override fun sizeOf(key: String, value: Bitmap): Int = value.byteCount / 1024
    }

    // Разжатый Lottie JSON: выбирается по одному, держим несколько последних.
    private val jsons = LruCache<String, String>(8)

    /** Уменьшение при чтении PNG: наибольшая степень двойки, при которой сторона не меньше нужной. */
    fun sampleFor(srcPx: Int, wantPx: Int): Int {
        var s = 1
        while (wantPx > 0 && srcPx / (s * 2) >= wantPx) s *= 2
        return s
    }

    /** Первый кадр стикера, уменьшенный до wantPx (512 → 256 или 128). null — файла нет или не читается. */
    fun preview(context: Context, id: String, wantPx: Int): Bitmap? {
        val key = "$id@$wantPx"
        previews.get(key)?.let { return it }
        val path = "$STICKERS/$id.png"
        return try {
            val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
            context.assets.open(path).use { BitmapFactory.decodeStream(it, null, bounds) }
            val opts = BitmapFactory.Options().apply { inSampleSize = sampleFor(maxOf(bounds.outWidth, bounds.outHeight), wantPx) }
            context.assets.open(path).use { BitmapFactory.decodeStream(it, null, opts) }?.also { previews.put(key, it) }
        } catch (e: Exception) {
            null
        }
    }

    /** Lottie JSON стикера: .tgs — это gzip, разжимаем сами (не полагаемся на распознавание библиотекой). */
    fun json(context: Context, id: String): String? {
        jsons.get(id)?.let { return it }
        return try {
            context.assets.open("$STICKERS/$id.tgs").use { raw ->
                GZIPInputStream(raw).bufferedReader(Charsets.UTF_8).use { it.readText() }
            }.also { jsons.put(id, it) }
        } catch (e: Exception) {
            null
        }
    }
}

/** Превью стикера (PNG первого кадра); пока читается — пустое место того же размера. */
@Composable
fun StickerPreview(id: String, size: Dp, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val px = with(LocalDensity.current) { size.roundToPx() }
    val bitmap by produceState<Bitmap?>(initialValue = null, id, px) {
        value = withContext(Dispatchers.IO) { MessageAssets.preview(context.applicationContext, id, px) }
    }
    Box(modifier.size(size), contentAlignment = Alignment.Center) {
        bitmap?.let { Image(it.asImageBitmap(), contentDescription = null, modifier = Modifier.size(size)) }
    }
}

/**
 * Движущийся стикер (выбранная клетка): Lottie по кругу. Пока JSON читается и разбирается —
 * превью, чтобы клетка не мигала пустотой.
 */
@Composable
fun StickerAnimated(id: String, size: Dp, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val json by produceState<String?>(initialValue = null, id) {
        value = withContext(Dispatchers.IO) { MessageAssets.json(context.applicationContext, id) }
    }
    val text = json
    if (text == null) {
        StickerPreview(id, size, modifier)
        return
    }
    val composition by rememberLottieComposition(LottieCompositionSpec.JsonString(text), cacheKey = "sticker-$id")
    Box(modifier.size(size), contentAlignment = Alignment.Center) {
        if (composition == null) {
            StickerPreview(id, size)
        } else {
            LottieAnimation(
                composition = composition,
                iterations = LottieConstants.IterateForever,
                modifier = Modifier.size(size),
            )
        }
    }
}
