package ru.atlas404.pcblock.crypto

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import ru.atlas404.pcblock.proto.Signer
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.Signature
import java.security.spec.ECGenParameterSpec

/**
 * Ключ телефона EC P-256 в Android Keystore (протокол, раздел 2). Закрытый ключ не покидает
 * Keystore; наружу — только открытый SubjectPublicKeyInfo DER (PublicKey.getEncoded()).
 */
class PhoneKey private constructor(private val priv: PrivateKey, override val publicDer: ByteArray) : Signer {

    override fun sign(data: ByteArray): ByteArray {
        val s = Signature.getInstance("SHA256withECDSA")
        s.initSign(priv)
        s.update(data)
        return s.sign()
    }

    companion object {
        private const val STORE = "AndroidKeyStore"
        private const val ALIAS = "pcblock-phone"

        /** Новая пара ключей для новой привязки; прежняя удаляется. */
        fun create(): PhoneKey {
            forget()
            val gen = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, STORE)
            gen.initialize(
                KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .build(),
            )
            val kp = gen.generateKeyPair()
            return PhoneKey(kp.private, kp.public.encoded)
        }

        /** Ключ привязки, если он есть. */
        fun load(): PhoneKey? = try {
            val ks = KeyStore.getInstance(STORE).apply { load(null) }
            val priv = ks.getKey(ALIAS, null) as? PrivateKey
            val pub = ks.getCertificate(ALIAS)?.publicKey
            if (priv != null && pub != null) PhoneKey(priv, pub.encoded) else null
        } catch (e: Exception) {
            null
        }

        /** Забыть ключ (отключение телефона, «Не совпадает»). */
        fun forget() {
            try {
                val ks = KeyStore.getInstance(STORE).apply { load(null) }
                if (ks.containsAlias(ALIAS)) ks.deleteEntry(ALIAS)
            } catch (e: Exception) {
                // Ключа нет или хранилище недоступно: забывать нечего.
            }
        }
    }
}
