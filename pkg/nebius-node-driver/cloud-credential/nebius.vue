<script>
import Banner from "@components/Banner/Banner";
import { LabeledInput } from "@components/Form/LabeledInput";

import {
  AUTH_ENDPOINT,
  DEFAULT_API_ENDPOINT,
  endpointHost,
  errorText,
  exchangeToken,
  isHostAllowed,
  signServiceAccountJWT,
} from "../utils/nebius";

export default {
  components: {
    Banner,
    LabeledInput,
  },

  props: {
    mode: {
      type:     String,
      required: true,
    },
    value: {
      type:     Object,
      required: true,
    },
  },

  async fetch() {
    try {
      this.driver = await this.$store.dispatch("rancher/find", {
        type: "nodedriver",
        id:   "nebius",
      });
    } catch (e) {
      this.driver = {};
    }

    this.validate();
  },

  data() {
    return {
      driver:  {},
      newKey:  "",
      busy:    false,
      error:   "",
      success: "",
    };
  },

  created() {
    this.value.setData(
      "projectId",
      this.value.decodedData.projectId ?? ""
    );
    this.value.setData(
      "apiEndpoint",
      this.value.decodedData.apiEndpoint || DEFAULT_API_ENDPOINT
    );

    // Never write the stored private key back into the form.
    this.validate();
  },

  computed: {
    disabled() {
      return this.mode === "view";
    },

    hasStoredKey() {
      return !!this.value?.decodedData?.privateKey;
    },

    apiHost() {
      return endpointHost(this.value.decodedData.apiEndpoint);
    },

    apiEndpointAllowed() {
      return isHostAllowed(
        this.driver,
        this.value.decodedData.apiEndpoint
      );
    },

    authEndpointAllowed() {
      return isHostAllowed(this.driver, AUTH_ENDPOINT);
    },

    endpointsAllowed() {
      return this.apiEndpointAllowed && this.authEndpointAllowed;
    },

    blockedHosts() {
      const hosts = [];

      if (!this.apiEndpointAllowed && this.apiHost) {
        hosts.push(this.apiHost);
      }
      if (!this.authEndpointAllowed) {
        hosts.push(endpointHost(AUTH_ENDPOINT));
      }

      return hosts;
    },

    canVerify() {
      return (
        !this.disabled &&
        !!this.value.decodedData.saId &&
        !!this.value.decodedData.publicKeyId &&
        (!!this.newKey || this.hasStoredKey) &&
        !!this.value.decodedData.projectId
      );
    },
  },

  methods: {
    validate() {
      if (
        !this.value.decodedData.saId ||
        !this.value.decodedData.publicKeyId ||
        (!this.newKey && !this.hasStoredKey) ||
        !this.value.decodedData.projectId ||
        !this.value.decodedData.apiEndpoint ||
        !this.endpointsAllowed
      ) {
        this.$emit("validationChanged", false);
        return;
      }

      if (this.newKey) {
        if (!/-----BEGIN (RSA )?PRIVATE KEY-----/.test(this.newKey)) {
          this.$emit("validationChanged", false);
          return;
        }
      }

      this.$emit("validationChanged", true);
    },

    onKeyInput(event) {
      this.newKey = event.target.value;
      this.value.setData("privateKey", this.newKey);
      this.error = "";
      this.success = "";
      this.validate();
    },

    async addHostsToAllowList() {
      const driver = this.driver;
      const domains = driver.whitelistDomains || driver.spec?.whitelistDomains || [];

      for (const host of this.blockedHosts) {
        if (!domains.includes(host)) {
          domains.push(host);
        }
      }

      driver.whitelistDomains = domains;

      try {
        await driver.save();
        this.error = "";
      } catch (e) {
        this.error = errorText(this.t, e, "cluster.nebius.errors.listsUnavailable");
      } finally {
        this.validate();
      }
    },

    async verify() {
      this.error = "";
      this.success = "";

      if (!this.canVerify) {
        this.error = this.t("cluster.credential.nebius.errors.missingRequired");
        return;
      }

      if (!this.endpointsAllowed) {
        this.error = this.t("cluster.nebius.errors.notAllowed", {
          hostname: this.blockedHosts.join(", "),
        });
        return;
      }

      this.busy = true;

      try {
        const privateKey = this.newKey || this.value.decodedData.privateKey;

        // The stored key is used only for signing; it is never rendered.
        if (!privateKey) {
          throw new Error("invalid-key");
        }

        const assertion = await signServiceAccountJWT({
          privateKey,
          saId:       this.value.decodedData.saId,
          publicKeyId: this.value.decodedData.publicKeyId,
        });

        await exchangeToken(this.$store, { assertion });

        this.success = this.t("cluster.credential.nebius.verified");
      } catch (e) {
        if (e?.message === "invalid-key") {
          this.error = this.t("cluster.nebius.errors.invalidPrivateKey");
        } else if (e?.message === "pkcs1-key") {
          this.error = this.t("cluster.nebius.errors.pkcs1PrivateKey");
        } else if (e?.message === "no-token") {
          this.error = this.t("cluster.nebius.errors.tokenResponse");
        } else {
          this.error = errorText(
            this.t,
            e,
            "cluster.nebius.errors.listsUnavailable"
          );
        }
      } finally {
        this.busy = false;
      }
    },
  },
};
</script>

<template>
  <div>
    <div class="row mb-20">
      <div class="col span-6">
        <LabeledInput
          :mode="mode"
          :disabled="disabled"
          :value="value.decodedData.saId"
          label-key="cluster.credential.nebius.saId.label"
          placeholder-key="cluster.credential.nebius.saId.placeholder"
          required
          @change="
            (e) => {
              value.setData('saId', e.target.value);
              validate();
            }
          "
        />
      </div>
      <div class="col span-6">
        <LabeledInput
          :mode="mode"
          :disabled="disabled"
          :value="value.decodedData.publicKeyId"
          label-key="cluster.credential.nebius.publicKeyId.label"
          placeholder-key="cluster.credential.nebius.publicKeyId.placeholder"
          required
          @change="
            (e) => {
              value.setData('publicKeyId', e.target.value);
              validate();
            }
          "
        />
      </div>
    </div>

    <div class="mb-20">
      <LabeledInput
        type="multiline"
        :mode="mode"
        :disabled="disabled"
        :value="newKey"
        :placeholder="
          hasStoredKey
            ? t('cluster.credential.nebius.privateKey.stored')
            : t('cluster.credential.nebius.privateKey.placeholder')
        "
        label-key="cluster.credential.nebius.privateKey.label"
        tooltip-key="cluster.credential.nebius.privateKey.tooltip"
        required
        @change="onKeyInput"
      />
    </div>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledInput
          :mode="mode"
          :disabled="disabled"
          :value="value.decodedData.projectId"
          label-key="cluster.credential.nebius.projectId.label"
          placeholder-key="cluster.credential.nebius.projectId.placeholder"
          required
          @change="
            (e) => {
              value.setData('projectId', e.target.value);
              validate();
            }
          "
        />
      </div>
    </div>

    <div class="row mb-20">
      <div class="col span-6">
        <LabeledInput
          :mode="mode"
          :disabled="disabled"
          :value="value.decodedData.apiEndpoint"
          label-key="cluster.credential.nebius.apiEndpoint.label"
          tooltip-key="cluster.credential.nebius.apiEndpoint.tooltip"
          required
          @change="
            (e) => {
              value.setData('apiEndpoint', e.target.value);
              validate();
            }
          "
        />
      </div>
    </div>

    <button
      class="btn role-primary"
      :disabled="busy || !canVerify"
      @click="verify"
    >
      {{
        busy
          ? t("cluster.credential.nebius.actions.verifying")
          : t("cluster.credential.nebius.actions.verify")
      }}
    </button>

    <Banner v-if="success" class="mt-20" color="success">
      {{ success }}
    </Banner>

    <Banner v-if="error" class="mt-20" color="error">
      {{ error }}
    </Banner>

    <Banner v-if="!endpointsAllowed" class="mt-20" color="warning">
      <div>
        {{
          t("cluster.nebius.errors.notAllowed", {
            hostname: blockedHosts.join(", "),
          })
        }}
      </div>
      <button class="btn ml-10 role-primary" @click="addHostsToAllowList">
        {{ t("cluster.credential.nebius.actions.addToAllowList") }}
      </button>
    </Banner>
  </div>
</template>

<style lang="scss" scoped>
</style>
