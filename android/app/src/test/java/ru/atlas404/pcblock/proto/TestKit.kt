package ru.atlas404.pcblock.proto

import org.json.JSONObject
import java.io.File
import java.security.KeyFactory
import java.security.PrivateKey
import java.security.Signature
import java.security.spec.PKCS8EncodedKeySpec
import javax.xml.parsers.DocumentBuilderFactory

/** Векторы Go (testdata/remote/vectors.json — ресурс тестов, см. app/build.gradle.kts). */
object Vectors {
    val json: JSONObject by lazy {
        val stream = Vectors::class.java.classLoader!!.getResourceAsStream("vectors.json")
            ?: error("vectors.json не найден среди ресурсов тестов")
        JSONObject(stream.bufferedReader(Charsets.UTF_8).readText())
    }

    fun s(name: String): String = json.getString(name)
    fun b(name: String): ByteArray = B64.dec(s(name))
    fun signed(name: String): Signed = json.getJSONObject(name).let { Signed(it.getString("data"), it.getString("sig")) }

    const val PC = "pc-vector"
    const val PHONE = "phone-vector"

    fun peer(): Proto.Peer = Proto.Peer(PC, PHONE, Crypto.parsePublicB64(s("pcPub")))
}

/** Программный ключ из PKCS#8 (в приложении ключ живёт в Android Keystore). */
class SoftSigner(privB64: String, pubB64: String) : Signer {
    private val priv: PrivateKey = KeyFactory.getInstance("EC").generatePrivate(PKCS8EncodedKeySpec(B64.dec(privB64)))
    override val publicDer: ByteArray = B64.dec(pubB64)

    override fun sign(data: ByteArray): ByteArray {
        val s = Signature.getInstance("SHA256withECDSA")
        s.initSign(priv)
        s.update(data)
        return s.sign()
    }

    companion object {
        fun pc() = SoftSigner(Vectors.s("pcPriv"), Vectors.s("pcPub"))
        fun phone() = SoftSigner(Vectors.s("phonePriv"), Vectors.s("phonePub"))
    }
}

/** Тексты приложения из res/values/strings.xml (рабочая папка тестов Gradle — папка модуля app). */
object Strings {
    val file: File by lazy {
        listOf("src/main/res/values/strings.xml", "app/src/main/res/values/strings.xml", "android/app/src/main/res/values/strings.xml")
            .map(::File).firstOrNull { it.exists() } ?: error("strings.xml не найден")
    }

    val all: Map<String, String> by lazy {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file)
        val nodes = doc.getElementsByTagName("string")
        (0 until nodes.length).associate { i ->
            val n = nodes.item(i)
            n.attributes.getNamedItem("name").nodeValue to n.textContent
        }
    }

    operator fun get(name: String): String = all[name] ?: error("нет строки $name")

    /** Пункты string-array (например, months_gen). */
    fun array(name: String): List<String> {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file)
        val arrays = doc.getElementsByTagName("string-array")
        for (i in 0 until arrays.length) {
            val a = arrays.item(i) as org.w3c.dom.Element
            if (a.getAttribute("name") != name) continue
            val items = a.getElementsByTagName("item")
            return (0 until items.length).map { items.item(it).textContent }
        }
        error("нет массива $name")
    }
}
