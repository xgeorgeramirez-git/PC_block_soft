package ru.atlas404.pcblock.proto

import java.math.BigInteger
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.PublicKey
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.X509EncodedKeySpec
import java.util.Base64

/** Ошибка протокола: данные не разобраны или не прошли проверку. */
class ProtoException(message: String) : Exception(message)

/** base64url без «=» (протокол, раздел 2). Разбор строгий: «=», «+», «/» и пробелы отвергаются. */
object B64 {
    private val enc = Base64.getUrlEncoder().withoutPadding()
    private val dec = Base64.getUrlDecoder()

    fun enc(b: ByteArray): String = enc.encodeToString(b)

    fun dec(s: String): ByteArray {
        if (s.isEmpty()) return ByteArray(0)
        if (s.any { !(it in 'A'..'Z' || it in 'a'..'z' || it in '0'..'9' || it == '-' || it == '_') }) {
            throw ProtoException("base64url: недопустимый знак")
        }
        if (s.length % 4 == 1) throw ProtoException("base64url: неверная длина")
        val out = try {
            dec.decode(s)
        } catch (e: IllegalArgumentException) {
            throw ProtoException("base64url: ${e.message}")
        }
        // Строгость как в Go (RawURLEncoding.Strict): лишние биты в хвосте запрещены.
        if (enc(out) != s) throw ProtoException("base64url: лишние биты")
        return out
    }
}

/** Подпись телефона: ключ в Android Keystore (crypto/PhoneKey.kt) или программный в тестах. */
interface Signer {
    /** Открытый ключ SubjectPublicKeyInfo DER (Java PublicKey.getEncoded()). */
    val publicDer: ByteArray

    /** Подпись SHA256withECDSA, ASN.1 DER. */
    fun sign(data: ByteArray): ByteArray
}

/** Подписанный блок {"data","sig"}: base64url байтов JSON и base64url подписи DER. */
data class Signed(val data: String, val sig: String)

object Crypto {
    private const val COMMIT_LABEL = "pcblock-commit-v1"
    private const val SAS_LABEL = "pcblock-sas-v1"

    /** Порядок группы кривой P-256 (secp256r1): так отличаем её от других кривых. */
    private val P256_ORDER = BigInteger("FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551", 16)

    fun sha256(vararg parts: ByteArray): ByteArray {
        val md = MessageDigest.getInstance("SHA-256")
        for (p in parts) md.update(p)
        return md.digest()
    }

    /** Ключ компьютера из SPKI DER; только EC P-256. */
    fun parsePublic(der: ByteArray): PublicKey {
        val key = try {
            KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(der))
        } catch (e: Exception) {
            throw ProtoException("ключ не разобран")
        }
        if (key !is ECPublicKey || key.params.order != P256_ORDER) throw ProtoException("ключ не P-256")
        return key
    }

    fun parsePublicB64(s: String): PublicKey = parsePublic(B64.dec(s))

    /** Проверка SHA256withECDSA; любой сбой разбора подписи — «не сошлась». */
    fun verify(pub: PublicKey, data: ByteArray, sig: ByteArray): Boolean = try {
        val v = Signature.getInstance("SHA256withECDSA")
        v.initVerify(pub)
        v.update(data)
        v.verify(sig)
    } catch (e: Exception) {
        false
    }

    /** Подписанные байты блока, если подпись верна; иначе исключение. JSON разбирается только после. */
    fun open(pub: PublicKey, s: Signed): ByteArray {
        val data = B64.dec(s.data)
        val sig = B64.dec(s.sig)
        if (sig.isEmpty() || !verify(pub, data, sig)) throw ProtoException("подпись не сошлась")
        return data
    }

    fun seal(signer: Signer, data: ByteArray): Signed = Signed(B64.enc(data), B64.enc(signer.sign(data)))

    /** SHA-256("pcblock-commit-v1" ‖ pcPubDER ‖ phonePubDER ‖ nPc) (протокол, раздел 4). */
    fun commit(pcPub: ByteArray, phonePub: ByteArray, nPc: ByteArray): ByteArray =
        sha256(COMMIT_LABEL.toByteArray(Charsets.UTF_8), pcPub, phonePub, nPc)

    /** Число сверки: первые 4 байта SHA-256(метка ‖ …) big-endian по модулю 1 000 000, 6 цифр. */
    fun sas(pcPub: ByteArray, phonePub: ByteArray, nPc: ByteArray, nPhone: ByteArray): String {
        val h = sha256(SAS_LABEL.toByteArray(Charsets.UTF_8), pcPub, phonePub, nPc, nPhone)
        var v = 0L
        for (i in 0 until 4) v = (v shl 8) or (h[i].toLong() and 0xff)
        return String.format(java.util.Locale.ROOT, "%06d", v % 1_000_000)
    }

    /** base64url SHA-256 байтов data команды (поле cmd ответа cmdr). */
    fun hashData(data: ByteArray): String = B64.enc(sha256(data))

    /** Сравнение без утечки по времени. */
    fun same(a: ByteArray, b: ByteArray): Boolean = MessageDigest.isEqual(a, b)
}
